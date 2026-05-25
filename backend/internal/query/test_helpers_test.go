//go:build integration

package query_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
)

// dsn for the shared CH fixture; set by TestMain.
var dsn string

// fatalReporter routes chtest.StartCH failures to log.Fatalf since TestMain
// runs before any *testing.T exists. Satisfies chtest.FatalReporter.
type fatalReporter struct{}

func (fatalReporter) Helper()                                  {}
func (fatalReporter) Fatalf(format string, args ...interface{}) { log.Fatalf(format, args...) }

func TestMain(m *testing.M) {
	fixture := chtest.StartCH(fatalReporter{}, "20260525120000_create_traces_v1.sql")
	dsn = fixture.DSN

	code := m.Run()
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
