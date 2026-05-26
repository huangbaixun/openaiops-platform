//go:build integration

// Cross-tenant isolation integration harness (SLICE-1 T13 / AC #8).
//
// Boots:
//   - PG via dockertest (real schema applied via pressly/goose).
//   - CH via internal/chquery/chtest (real traces_v1 + Row Policy).
//   - In-process ingester: otlpreceiver (gRPC + HTTP) → Consumer → real
//     PG resolver + real chquery.Conn + Metering (best-effort PG insert).
//   - In-process query: chi router from query.NewRouter behind a real
//     httptest.Server so plain net/http calls exercise the full middleware
//     chain — same as production with Caddy's /api prefix stripped.
//
// Lives under internal/ingest because the goal is end-to-end coverage of
// the receiver→consumer→CH path; the query side is a thin verification
// harness.
package ingest_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingest"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

// Package-level state owned by TestMain. Tests inside the package consume
// these via bringUpIngestAndQuery / SeedTenant.
var (
	pgDSN     string
	pgPool    *sql.DB
	chFix     *chtest.Fixture
	chConnURL string
)

// ingestEnv bundles the runtime that a single test case needs: ingester
// listen addresses, the query base URL, and direct PG/CH handles for
// seeding and polling. Shutdown stops the receiver, closes the query
// server, and drains/closes the metering goroutine + CH conn.
type ingestEnv struct {
	IngestGRPCAddr string
	IngestHTTPAddr string
	QueryBaseURL   string
	CHConn         *chquery.Conn
	PG             *sql.DB
	Shutdown       func()
}

// SeedTenant inserts a tenant + api_key row in PG and returns
// (tenant_id, bearer_plaintext). Plaintexts are random per call so
// repeated SeedTenant("acme") in different tests don't collide on the
// unique hashed_key index.
func (e *ingestEnv) SeedTenant(t *testing.T, name string) (uuid.UUID, string) {
	t.Helper()
	tid := uuid.New()
	_, err := e.PG.ExecContext(context.Background(),
		`INSERT INTO tenants (id, name, plan) VALUES ($1, $2, 'free')`,
		tid, name)
	require.NoError(t, err)

	plaintext := "test-key-" + name + "-" + randHex(4)
	hashed, err := apikey.Hash(plaintext)
	require.NoError(t, err)
	_, err = e.PG.ExecContext(context.Background(),
		`INSERT INTO api_keys (tenant_id, name, hashed_key, scope) VALUES ($1, $2, $3, 'read-write')`,
		tid, name+"-primary", hashed)
	require.NoError(t, err)
	return tid, plaintext
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	// ---- PG via dockertest ----
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("docker pool: %s", err)
	}
	pg, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=openaiops",
			"POSTGRES_PASSWORD=openaiops",
			"POSTGRES_DB=openaiops",
		},
	}, func(c *docker.HostConfig) { c.AutoRemove = true })
	if err != nil {
		log.Fatalf("pg start: %s", err)
	}
	pgPort := pg.GetPort("5432/tcp")
	pgDSN = fmt.Sprintf("postgres://openaiops:openaiops@localhost:%s/openaiops?sslmode=disable", pgPort)
	pool.MaxWait = 60 * time.Second
	if err := pool.Retry(func() error {
		db, err := sql.Open("pgx", pgDSN)
		if err != nil {
			return err
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return db.PingContext(ctx)
	}); err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("pg ping: %s", err)
	}
	pgPool, err = sql.Open("pgx", pgDSN)
	if err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("pg open: %s", err)
	}

	// Apply PG migrations via goose against the test's migrations dir.
	// goose path is relative to the package under test, matching the
	// pattern in internal/auth/resolver_pg_test.go.
	if err := goose.SetDialect("postgres"); err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("goose dialect: %s", err)
	}
	if err := goose.Up(pgPool, "../../migrations"); err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("goose up: %s", err)
	}

	// ---- CH via chquery/chtest ----
	chFix = chtest.StartCH(stderrFatal{}, "20260525120000_create_traces_v1.sql")
	chConnURL = chFix.DSN

	code := m.Run()

	if pgPool != nil {
		_ = pgPool.Close()
	}
	if chFix != nil {
		_ = chFix.Close()
	}
	_ = pool.Purge(pg)
	os.Exit(code)
}

// stderrFatal adapts log.Fatalf into chtest.FatalReporter for the
// TestMain-time CH startup (no *testing.T available yet).
type stderrFatal struct{}

func (stderrFatal) Helper()                   {}
func (stderrFatal) Fatalf(f string, a ...any) { log.Fatalf(f, a...) }

// pickPort returns "127.0.0.1:N" with a free N. Used to keep the
// ingester listeners off any fixed port so the test can run alongside
// a dev stack on :4317/:4318.
func pickPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// bringUpIngestAndQuery wires the in-process system under test: real
// PG resolver + real CH conn + real Consumer + real otlpreceiver
// (gRPC + HTTP) on random ports; and a real query.NewRouter behind
// httptest.Server. Caller must defer env.Shutdown().
func bringUpIngestAndQuery(t *testing.T) *ingestEnv {
	t.Helper()

	chConn, err := chquery.Connect(context.Background(), chConnURL)
	require.NoError(t, err)

	resolver := auth.NewPGResolver(pgPool)
	reg := prometheus.NewRegistry()
	base := ingestshared.NewBaseMetrics(reg, "trace")
	metrics := ingest.NewMetrics(reg, base)
	metering := ingestshared.NewMetering(pgPool, base, "trace")
	consumer := ingest.NewConsumer(resolver, chConn, metering, metrics)

	grpcAddr := pickPort(t)
	httpAddr := pickPort(t)

	rcvr, err := ingest.NewOTLPReceiver(ingest.ReceiverConfig{
		GRPCAddr: grpcAddr,
		HTTPAddr: httpAddr,
	}, consumer)
	require.NoError(t, err)
	require.NoError(t, rcvr.Start(context.Background(), ingestshared.NewHost()))

	qrouter := query.NewRouter(resolver, chConn)
	qsrv := httptest.NewServer(qrouter)

	return &ingestEnv{
		IngestGRPCAddr: grpcAddr,
		IngestHTTPAddr: httpAddr,
		QueryBaseURL:   qsrv.URL,
		CHConn:         chConn,
		PG:             pgPool,
		Shutdown: func() {
			qsrv.Close()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = rcvr.Shutdown(shutdownCtx)
			metering.Drain(shutdownCtx)
			metering.Close()
			_ = chConn.Close()
		},
	}
}

// pollCH polls CH until tenant `tid` has at least `want` rows in
// traces_v1, or `timeout`. Uses authCtxFor so chquery's MustTenantScope
// is satisfied — bypassing it would either panic or false-pass.
func pollCH(t *testing.T, ch *chquery.Conn, tid uuid.UUID, want uint64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ctx := authCtxFor(tid.String())
	var last uint64
	for time.Now().Before(deadline) {
		var n uint64
		if err := ch.QueryRow(ctx,
			`SELECT count() FROM traces_v1 WHERE tenant_id = ?`).Scan(&n); err == nil {
			last = n
			if n >= want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("CH did not show >=%d rows for tenant %s within %s (last=%d)",
		want, tid, timeout, last)
}

// callList GETs /v1/traces with optional bearer. baseURL is the
// httptest.Server URL — no /api prefix (Caddy strips it in prod).
// path may include the query string; pass "/v1/traces" plain or
// "/v1/traces?ts_from=...&ts_to=..." to widen the ts window.
func callList(baseURL, path, bearer string) (statusCode int, body []byte, err error) {
	if path == "" {
		path = "/v1/traces"
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

// callDetail GETs /v1/traces/{traceID} with optional bearer.
func callDetail(baseURL, bearer, traceID string) (statusCode int) {
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/traces/"+traceID, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// authCtxFor wraps auth.WithTenant for callers that have the tenant_id
// only in string form (e.g., pollCH). The test poller has nothing to
// do with HTTP middleware so we stamp the tenant into ctx manually.
func authCtxFor(tidStr string) context.Context {
	tid := uuid.MustParse(tidStr)
	return auth.WithTenant(context.Background(), tid, "poll")
}

// bytesReader returns an io.ReadCloser over b. Used to construct the
// OTLP/HTTP request body without taking a runtime dep on a buffer pool.
func bytesReader(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
