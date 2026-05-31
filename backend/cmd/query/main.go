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
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
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
		logger.Error("CLICKHOUSE_DSN is required for query")
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
	router := query.NewRouter(resolver, ch, db)

	pruneCtx, prunecancel := context.WithCancel(context.Background())
	pruner := query.NewAnnotationsPruner(query.NewAnnotationsRepo(db),
		cfg.AnnotationsRetentionDays, cfg.AnnotationsPruneInterval)
	go pruner.Run(pruneCtx)

	srv := &http.Server{
		Addr:              cfg.QueryListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("query listening", "addr", cfg.QueryListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	prunecancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}
