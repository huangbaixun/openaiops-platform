//go:build integration

package topoengine_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
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
