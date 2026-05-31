// topo-engine is the SLICE-3 background aggregator binary. Every TickInterval
// it discovers active tenants via the PG tenants table and aggregates the
// previous closed bucket into topology_edges_v1 + service_stats_v1 using a
// tenant-scoped chquery.Conn.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/config"
	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("topo-engine", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.ClickHouseDSN == "" {
		return errors.New("CLICKHOUSE_DSN is required for topo-engine")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pg open: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		return fmt.Errorf("pg ping: %w", err)
	}

	ch, err := chquery.Connect(context.Background(), cfg.ClickHouseDSN)
	if err != nil {
		return fmt.Errorf("ch connect: %w", err)
	}
	defer ch.Close()

	// Dedicated registry so topo-engine /metrics only exposes its own series
	// (rather than the process-default Go runtime metrics colliding with
	// other binaries reusing the shared image).
	reg := prometheus.NewRegistry()
	metrics := topoengine.NewMetrics(reg)
	// Tenant discovery reads the PG tenants table (ADR-0005); no Row-Policy-exempt CH user needed.
	eng := topoengine.New(
		topoengine.Config{
			TickInterval:      cfg.TopoTickInterval,
			CatchupMax:        cfg.TopoCatchupMax,
			TenantConcurrency: cfg.TopoTenantConcurrency,
		},
		topoengine.Deps{CH: ch, PG: db},
		metrics,
	)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Admin server: /healthz, /livez, /metrics. /healthz flips to 200 only
	// after Catchup completes (operational readiness gate — keeps the
	// container "starting" while backfilling the catchup window).
	var ready atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("warming up"))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:              cfg.TopoEngineAdminAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("topo-engine admin listening", "addr", cfg.TopoEngineAdminAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Catchup runs once on boot, then ready flips and the periodic tick
	// loop takes over.
	go func() {
		if err := eng.Catchup(rootCtx); err != nil {
			logger.Error("topo-engine catchup", "err", err)
		}
		ready.Store(true)
		logger.Info("topo-engine catchup complete; ticker starting")
	}()

	go func() {
		ticker := time.NewTicker(cfg.TopoTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-rootCtx.Done():
				return
			case t := <-ticker.C:
				bucket := topoengine.ClosedBucketAt(t)
				if err := eng.RunBucket(rootCtx, bucket); err != nil {
					logger.Error("topo-engine tick", "bucket", bucket, "err", err)
					continue
				}
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case <-quit:
		logger.Info("topo-engine shutting down on signal")
	case err := <-errCh:
		runErr = fmt.Errorf("admin listen: %w", err)
	}

	cancel()
	shutdownCtx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("admin shutdown: %w", shutdownErr)
	}
	logger.Info("topo-engine shutdown complete")
	return runErr
}
