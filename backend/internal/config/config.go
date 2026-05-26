package config

import (
	"errors"
	"os"
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
}

func FromEnv() (Config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return Config{
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
	}, nil
}

func defaultAddr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}
