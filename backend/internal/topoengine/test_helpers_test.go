//go:build integration

package topoengine_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

// SpanSpec is the per-span seed shape used by topoengine integration tests.
// It is deliberately small: only the columns Pass A / Pass B SQL reads from
// (service, parent linkage, kind, status, duration, attributes).
type SpanSpec struct {
	Service      string
	SpanID       string
	ParentSpanID string
	Kind         string
	Status       string
	DurationNs   uint64
	Attrs        map[string]string
}

// chDSN is the DSN for the ephemeral CH instance started by TestMain.
var chDSN string

// fatalReporter routes chtest.StartCH failures to log.Fatalf since TestMain
// runs before any *testing.T exists. Satisfies chtest.FatalReporter.
type fatalReporter struct{}

func (fatalReporter) Helper()                                  {}
func (fatalReporter) Fatalf(format string, args ...interface{}) { log.Fatalf(format, args...) }

// TestMain spins up CH once for the whole topoengine integration suite and
// applies the three CH migrations Pass A + Pass B depend on.
func TestMain(m *testing.M) {
	fix := chtest.StartCH(fatalReporter{},
		"20260525120000_create_traces_v1.sql",
		"20260527120000_create_topology_edges_v1.sql",
		"20260527120100_create_service_stats_v1.sql",
	)
	chDSN = fix.DSN
	code := m.Run()
	_ = fix.Close()
	os.Exit(code)
}

// setupCH opens a chquery.Conn against the shared CH fixture.
func setupCH(t *testing.T) *chquery.Conn {
	t.Helper()
	c, err := chquery.Connect(context.Background(), chDSN)
	require.NoError(t, err)
	return c
}

// setupEngine wires a topoengine.Engine on top of the shared CH fixture.
// PG is nil — Pass A / Pass B don't need it. Future tests that exercise
// idempotency state will pass a *sql.DB via Deps.PG.
func setupEngine(t *testing.T, cfg topoengine.Config) (*topoengine.Engine, *chquery.Conn) {
	t.Helper()
	conn := setupCH(t)
	admin := chquery.NewAdminConn(conn)
	reg := prometheus.NewRegistry()
	metrics := topoengine.NewMetrics(reg)
	eng := topoengine.New(cfg, topoengine.Deps{CH: conn, Admin: admin, PG: nil}, metrics)
	return eng, conn
}

// setupEngineWithPG wires an engine whose Deps.PG points at the shared ephemeral
// Postgres, so activeTenants (PG-driven, PLATFORM-TOPO-1) can be exercised.
func setupEngineWithPG(t *testing.T, cfg topoengine.Config, db *sql.DB) (*topoengine.Engine, *chquery.Conn) {
	t.Helper()
	conn := setupCH(t)
	reg := prometheus.NewRegistry()
	metrics := topoengine.NewMetrics(reg)
	eng := topoengine.New(cfg, topoengine.Deps{CH: conn, PG: db}, metrics)
	return eng, conn
}

func timeNowUTC() time.Time { return time.Now().UTC() }

func authCtx(tid uuid.UUID) context.Context {
	return auth.WithTenant(context.Background(), tid, "test")
}

// seedSpansForTenant inserts spans for the given tenant into traces_v1.
// All spans share the same trace_id (derived from tenant prefix) and are
// stamped at bucket+(i+1)*100ms so they all fall in [bucket, bucket+1min).
//
// Uses auth.WithTenant(ctx, uuid.UUID, name) — 3-arg signature confirmed
// against backend/internal/auth/ctx.go.
func seedSpansForTenant(t *testing.T, conn *chquery.Conn, tidStr string, bucket time.Time, specs []SpanSpec) string {
	t.Helper()
	tid := uuid.MustParse(tidStr)
	ctx := auth.WithTenant(context.Background(), tid, "test-tenant-"+tidStr[:8])
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO traces_v1 (tenant_id, trace_id, span_id, parent_span_id, service, operation,
            ts, duration, status, span_kind, resource_attributes, attributes) VALUES`)
	require.NoError(t, err)
	traceID := "trace-" + tidStr[:8]
	for i, s := range specs {
		require.NoError(t, batch.Append(
			tidStr, traceID, s.SpanID, s.ParentSpanID,
			s.Service, "op",
			bucket.Add(time.Duration(i+1)*100*time.Millisecond),
			s.DurationNs, s.Status, s.Kind,
			map[string]string{}, mapOr(s.Attrs, map[string]string{}),
		))
	}
	require.NoError(t, batch.Send())
	return traceID
}

func mapOr(m, dflt map[string]string) map[string]string {
	if m == nil {
		return dflt
	}
	return m
}

// edgeRow is the projection of topology_edges_v1 columns the Pass A tests
// assert against. Errors and p95 are exposed for richer assertions later.
type edgeRow struct {
	Caller     string
	CallerKind string
	Callee     string
	CalleeKind string
	Calls      uint64
	Errors     uint64
	P95        uint64
}

// queryEdges returns all topology_edges_v1 rows for the tenant in ctx + bucket
// via FINAL. The empty-string placeholder is consumed by MustTenantScope and
// replaced with the tenant_id from ctx (see chquery/scope.go); bucket is the
// real second argument the query expects.
func queryEdges(t *testing.T, conn *chquery.Conn, ctx context.Context, bucket time.Time) []edgeRow {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT caller_service, caller_kind, callee_service, callee_kind, calls, errors, p95_duration
         FROM topology_edges_v1 FINAL
         WHERE tenant_id = ? AND ts_bucket = ?
         ORDER BY caller_service, callee_service, callee_kind`,
		bucket)
	require.NoError(t, err)
	defer rows.Close()
	var out []edgeRow
	for rows.Next() {
		var e edgeRow
		require.NoError(t, rows.Scan(&e.Caller, &e.CallerKind, &e.Callee, &e.CalleeKind, &e.Calls, &e.Errors, &e.P95))
		out = append(out, e)
	}
	return out
}

// statsRow is the projection of service_stats_v1 columns the Pass B tests
// assert against.
type statsRow struct {
	Service, SpanKind  string
	Calls, Errors, P95 uint64
}

// queryStats returns all service_stats_v1 rows for the tenant + bucket via FINAL.
// MustTenantScope prepends tenant_id from ctx; only bucket is explicit.
func queryStats(t *testing.T, conn *chquery.Conn, ctx context.Context, bucket time.Time) []statsRow {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT service, span_kind, calls, errors, p95_duration
         FROM service_stats_v1 FINAL
         WHERE tenant_id = ? AND ts_bucket = ?
         ORDER BY service, span_kind`,
		bucket)
	require.NoError(t, err)
	defer rows.Close()
	var out []statsRow
	for rows.Next() {
		var s statsRow
		require.NoError(t, rows.Scan(&s.Service, &s.SpanKind, &s.Calls, &s.Errors, &s.P95))
		out = append(out, s)
	}
	return out
}

// ---- PG fixture (lazy, on-demand for cross-tenant API tests) -----------

var (
	pgOnce     sync.Once
	pgDSN      string
	pgPool     *dockertest.Pool
	pgResource *dockertest.Resource
	pgInitErr  error
)

// pgFromDockertest starts a single ephemeral Postgres for the suite (lazy:
// pays the start cost only when an API-level test calls it) and applies the
// PG migrations via goose. Subsequent calls reuse the same DSN.
func pgFromDockertest(t *testing.T) string {
	t.Helper()
	pgOnce.Do(func() {
		pgPool, pgInitErr = dockertest.NewPool("")
		if pgInitErr != nil {
			return
		}
		pgResource, pgInitErr = pgPool.RunWithOptions(&dockertest.RunOptions{
			Repository: "postgres",
			Tag:        "16-alpine",
			Env: []string{
				"POSTGRES_PASSWORD=test",
				"POSTGRES_DB=test",
			},
		}, func(c *docker.HostConfig) { c.AutoRemove = true })
		if pgInitErr != nil {
			return
		}
		pgDSN = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable",
			pgResource.GetPort("5432/tcp"))
		if pgInitErr = pgPool.Retry(func() error {
			db, err := sql.Open("pgx", pgDSN)
			if err != nil {
				return err
			}
			defer db.Close()
			return db.Ping()
		}); pgInitErr != nil {
			return
		}
		db, _ := sql.Open("pgx", pgDSN)
		defer db.Close()
		if pgInitErr = goose.SetDialect("postgres"); pgInitErr != nil {
			return
		}
		// migrations path relative to this test file: internal/topoengine/
		if pgInitErr = goose.Up(db, "../../migrations"); pgInitErr != nil {
			return
		}
	})
	require.NoError(t, pgInitErr, "pg fixture init")
	return pgDSN
}

// pgEnsureSchema opens a *sql.DB against the ephemeral PG and truncates
// tenants + api_keys so each top-level test starts from a known-empty state.
// Returns the open DB; caller is responsible for Close().
func pgEnsureSchema(t *testing.T) *sql.DB {
	t.Helper()
	dsn := pgFromDockertest(t)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(),
		"TRUNCATE TABLE api_keys, tenants RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	return db
}

// seedAPIKey inserts a tenant with the given fixed UUID + name, then inserts
// an api_key with the bcrypt-hashed plaintext. Plaintext is returned for use
// in the Bearer header. Using a fixed tenant UUID (rather than the PG-assigned
// one) makes the test's CH-side seed match what the resolver returns from PG.
func seedAPIKey(t *testing.T, db *sql.DB, tenantID uuid.UUID, tenantName, plaintext string) string {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		`INSERT INTO tenants(id, name) VALUES($1, $2)`, tenantID, tenantName)
	require.NoError(t, err)
	hashed, err := apikey.Hash(plaintext)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO api_keys(tenant_id, name, hashed_key, scope)
		 VALUES($1, $2, $3, 'read-write')`,
		tenantID, "primary", hashed)
	require.NoError(t, err)
	return plaintext
}

// startQueryServer mounts the production query router on a httptest.Server
// (auth.Middleware + PGResolver + chquery.Conn) and returns the server +
// base URL. Caller defers srv.Close().
func startQueryServer(t *testing.T, db *sql.DB, ch *chquery.Conn) *httptest.Server {
	t.Helper()
	resolver := auth.NewPGResolver(db)
	r := query.NewRouter(resolver, ch, db)
	return httptest.NewServer(r)
}

// mustGet issues GET path with optional Bearer (empty string skips header)
// and returns (status, body). Body is fully drained so the connection is reusable.
func mustGet(t *testing.T, baseURL, path, bearer string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
	require.NoError(t, err)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}
