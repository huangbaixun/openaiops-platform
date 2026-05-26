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

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/config"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
	"github.com/huangbaixun/openaiops-platform/backend/internal/logingest"
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

	resolver := auth.NewPGResolver(db)
	consumer := logingest.NewLogConsumer(resolver, ch, metering, base)
	rcvr, err := logingest.NewOTLPLogReceiver(logingest.ReceiverConfig{
		GRPCAddr: cfg.LogIngesterOTLPGRPCAddr,
		HTTPAddr: cfg.LogIngesterOTLPHTTPAddr,
	}, consumer)
	if err != nil {
		return fmt.Errorf("otlp log receiver build: %w", err)
	}
	if err := rcvr.Start(context.Background(), ingestshared.NewHost()); err != nil {
		return fmt.Errorf("otlp log receiver start: %w", err)
	}
	logger.Info("log-ingester otlp listening", "grpc", cfg.LogIngesterOTLPGRPCAddr, "http", cfg.LogIngesterOTLPHTTPAddr)

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
	// Drain real traffic first — OTLP receiver before admin so /healthz keeps
	// answering until the receiver has stopped accepting batches.
	if rcvrErr := rcvr.Shutdown(ctx); rcvrErr != nil && runErr == nil {
		runErr = fmt.Errorf("otlp log receiver shutdown: %w", rcvrErr)
	}
	// Receiver stopped → no new metering events. Flush whatever is pending in
	// the queue before tearing down admin + PG/CH connections.
	metering.Drain(ctx)
	if shutdownErr := adminSrv.Shutdown(ctx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("admin shutdown: %w", shutdownErr)
	}
	logger.Info("log-ingester shutdown complete")
	return runErr
}
