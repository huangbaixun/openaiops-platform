package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL             string
	ClickHouseDSN           string
	GatewayListenAddr       string
	QueryListenAddr         string
	IngesterOTLPGRPCAddr    string
	IngesterOTLPHTTPAddr    string
	IngesterAdminAddr       string
	LogIngesterOTLPGRPCAddr string
	LogIngesterOTLPHTTPAddr string
	LogIngesterAdminAddr    string

	// topo-engine (SLICE-3)
	TopoEngineAdminAddr   string        // default :8084
	TopoTickInterval      time.Duration // default 1m
	TopoCatchupMax        time.Duration // default 1h
	TopoTenantConcurrency int           // default 4
}

func FromEnv() (Config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	cfg := Config{
		DatabaseURL:             dsn,
		ClickHouseDSN:           os.Getenv("CLICKHOUSE_DSN"),
		GatewayListenAddr:       defaultAddr("GATEWAY_LISTEN_ADDR", ":8080"),
		QueryListenAddr:         defaultAddr("QUERY_LISTEN_ADDR", ":8081"),
		IngesterOTLPGRPCAddr:    defaultAddr("INGESTER_OTLP_GRPC_ADDR", "0.0.0.0:4317"),
		IngesterOTLPHTTPAddr:    defaultAddr("INGESTER_OTLP_HTTP_ADDR", "0.0.0.0:4318"),
		IngesterAdminAddr:       defaultAddr("INGESTER_ADMIN_ADDR", "0.0.0.0:8082"),
		LogIngesterOTLPGRPCAddr: defaultAddr("LOG_INGESTER_OTLP_GRPC_ADDR", "0.0.0.0:4327"),
		LogIngesterOTLPHTTPAddr: defaultAddr("LOG_INGESTER_OTLP_HTTP_ADDR", "0.0.0.0:4328"),
		LogIngesterAdminAddr:    defaultAddr("LOG_INGESTER_ADMIN_ADDR", "0.0.0.0:8083"),
		TopoEngineAdminAddr:     defaultAddr("TOPO_ENGINE_ADMIN_ADDR", ":8084"),
	}

	if v := os.Getenv("TOPO_TICK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("TOPO_TICK_INTERVAL: %w", err)
		}
		cfg.TopoTickInterval = d
	} else {
		cfg.TopoTickInterval = time.Minute
	}

	if v := os.Getenv("TOPO_CATCHUP_MAX"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("TOPO_CATCHUP_MAX: %w", err)
		}
		cfg.TopoCatchupMax = d
	} else {
		cfg.TopoCatchupMax = time.Hour
	}

	if v := os.Getenv("TOPO_TENANT_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("TOPO_TENANT_CONCURRENCY: must be positive int (got %q)", v)
		}
		cfg.TopoTenantConcurrency = n
	} else {
		cfg.TopoTenantConcurrency = 4
	}

	return cfg, nil
}

func defaultAddr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}
