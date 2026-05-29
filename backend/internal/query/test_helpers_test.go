//go:build integration

package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
)

// dsn for the shared CH fixture; set by TestMain.
var dsn string

var annotationsPGDSN string

// startPG is called from TestMain to provide a PG instance for the
// annotations repo integration tests (the rest of the package uses CH).
func startPG() func() {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("dockertest pool: %v", err)
	}
	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres", Tag: "16-alpine",
		Env: []string{"POSTGRES_PASSWORD=test", "POSTGRES_DB=test"},
	}, func(c *docker.HostConfig) { c.AutoRemove = true })
	if err != nil {
		log.Fatalf("dockertest pg run: %v", err)
	}
	annotationsPGDSN = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable",
		res.GetPort("5432/tcp"))
	if err := pool.Retry(func() error {
		db, _ := sql.Open("pgx", annotationsPGDSN)
		defer db.Close()
		return db.Ping()
	}); err != nil {
		log.Fatalf("pg not ready: %v", err)
	}
	return func() { _ = pool.Purge(res) }
}

// fatalReporter routes chtest.StartCH failures to log.Fatalf since TestMain
// runs before any *testing.T exists. Satisfies chtest.FatalReporter.
type fatalReporter struct{}

func (fatalReporter) Helper()                                  {}
func (fatalReporter) Fatalf(format string, args ...interface{}) { log.Fatalf(format, args...) }

func TestMain(m *testing.M) {
	fixture := chtest.StartCH(fatalReporter{},
		"20260525120000_create_traces_v1.sql",
		"20260527120000_create_logs_v1.sql",
		"20260527120000_create_topology_edges_v1.sql",
		"20260527120100_create_service_stats_v1.sql",
	)
	dsn = fixture.DSN
	stopPG := startPG()

	code := m.Run()
	stopPG()
	_ = fixture.Close()
	os.Exit(code)
}

// setupCH opens a chquery.Conn against the dockertest CH.
func setupCH(t *testing.T) *chquery.Conn {
	t.Helper()
	c, err := chquery.Connect(context.Background(), dsn)
	require.NoError(t, err)
	return c
}

// seedLog inserts a single log row under the tenant in ctx.
func seedLog(t *testing.T, conn *chquery.Conn, ctx context.Context, tidStr, service, severity, body, traceID string) {
	t.Helper()
	now := time.Now().UTC()
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO logs_v1 (
			tenant_id, ts, observed_ts, service, severity_text, severity_number,
			body, trace_id, span_id, trace_flags, resource_attributes, attributes
		) VALUES`)
	require.NoError(t, err)
	require.NoError(t, batch.Append(
		tidStr, now, now, service, severity, uint8(0),
		body, traceID, "", uint8(0),
		map[string]string{}, map[string]string{},
	))
	require.NoError(t, batch.Send())
}

// seedSpans inserts n spans of a single trace under tid. Shared between
// list and detail integration tests.
func seedSpans(t *testing.T, conn *chquery.Conn, ctx context.Context, tid uuid.UUID, traceID string, n int) {
	t.Helper()
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO traces_v1 (tenant_id, trace_id, span_id, parent_span_id, service, operation,
            ts, duration, status, span_kind, resource_attributes, attributes) VALUES`)
	require.NoError(t, err)
	now := time.Now().UTC()
	tidStr := tid.String()
	for i := 0; i < n; i++ {
		require.NoError(t, batch.Append(
			tidStr, traceID, fmt.Sprintf("span-%c", rune('a'+i)), "",
			"frontend", "GET /", now.Add(time.Duration(i)*time.Millisecond),
			uint64(100_000_000), "Ok", "Server",
			map[string]string{"host.name": "h1"},
			map[string]string{"http.status_code": "200"},
		))
	}
	require.NoError(t, batch.Send())
}
