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
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/config"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("log-ingester", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.ClickHouseDSN == "" {
		return errors.New("CLICKHOUSE_DSN is required for log-ingester")
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

	base := ingestshared.NewBaseMetrics(prometheus.DefaultRegisterer, "log")
	metering := ingestshared.NewMetering(db, base, "log")
	defer metering.Close()

	// Placeholders — T4 will wire the OTLP log receiver here.
	_ = ch
	_ = metering

	// Admin server.
	adminSrv := &http.Server{
		Addr:              cfg.LogIngesterAdminAddr,
		Handler:           ingestshared.AdminHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("log-ingester admin listening", "addr", cfg.LogIngesterAdminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case <-quit:
		logger.Info("log-ingester shutting down on signal")
	case err := <-errCh:
		runErr = fmt.Errorf("admin listen: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Flush pending metering events before tearing down connections.
	metering.Drain(ctx)
	if shutdownErr := adminSrv.Shutdown(ctx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("admin shutdown: %w", shutdownErr)
	}
	logger.Info("log-ingester shutdown complete")
	return runErr
}
