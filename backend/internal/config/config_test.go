package config

import "testing"

func TestFromEnv_LogIngesterDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("CLICKHOUSE_DSN", "clickhouse://x")
	// Clear shell-leaked values so defaults reliably apply (defaultAddr falls back when env=="").
	t.Setenv("LOG_INGESTER_OTLP_GRPC_ADDR", "")
	t.Setenv("LOG_INGESTER_OTLP_HTTP_ADDR", "")
	t.Setenv("LOG_INGESTER_ADMIN_ADDR", "")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.LogIngesterOTLPGRPCAddr != "0.0.0.0:4327" {
		t.Fatalf("grpc default: got %q", c.LogIngesterOTLPGRPCAddr)
	}
	if c.LogIngesterOTLPHTTPAddr != "0.0.0.0:4328" {
		t.Fatalf("http default: got %q", c.LogIngesterOTLPHTTPAddr)
	}
	if c.LogIngesterAdminAddr != "0.0.0.0:8083" {
		t.Fatalf("admin default: got %q", c.LogIngesterAdminAddr)
	}
}
