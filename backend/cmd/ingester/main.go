package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/config"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingest"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}
	if cfg.ClickHouseDSN == "" {
		logger.Error("CLICKHOUSE_DSN is required for ingester")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		logger.Error("pg open", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		logger.Error("pg ping", "err", err)
		os.Exit(1)
	}

	ch, err := chquery.Connect(context.Background(), cfg.ClickHouseDSN)
	if err != nil {
		logger.Error("ch connect", "err", err)
		os.Exit(1)
	}
	defer ch.Close()

	resolver := auth.NewPGResolver(db)
	_ = resolver // wired into the consumer in Task 5

	// Admin server first — OTLP receiver wiring lands in Task 4.
	adminSrv := &http.Server{
		Addr:              cfg.IngesterAdminAddr,
		Handler:           ingest.AdminHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("ingester admin listening", "addr", cfg.IngesterAdminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("admin listen", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = adminSrv.Shutdown(ctx)
	logger.Info("ingester shutdown complete")
}
