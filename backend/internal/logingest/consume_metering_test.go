//go:build integration

// TestSlice2_MeteringSignalLog is the SLICE-2 AC #9 gate:
// proves that ConsumeLogs writes a metering_events row with
// signal_type='log' and count=N after successfully processing N log records.
//
// Setup:
//   - PG via dockertest (real schema via pressly/goose).
//   - CH via internal/chquery/chtest (logs_v1 + Row Policy).
//   - In-process LogConsumer with real PG resolver, real chquery.Conn,
//     and real ingestshared.Metering.
//   - Bearer injected directly via client.NewContext (no full receiver
//     stack needed — we're testing the consumer, not the transport).
//
// This is AC #9 only; the cross-tenant E2E covering AC #8 for logs is T11.
package logingest_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
	"github.com/huangbaixun/openaiops-platform/backend/internal/logingest"

	"github.com/google/uuid"
)

// Package-level state owned by TestMain — mirrors the pattern from
// internal/ingest/cross_tenant_helpers_test.go (SLICE-1 T13).
var (
	meteringPGPool *sql.DB
	meteringCHFix  *chtest.Fixture
)

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
	pgDSN := fmt.Sprintf("postgres://openaiops:openaiops@localhost:%s/openaiops?sslmode=disable", pgPort)

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

	meteringPGPool, err = sql.Open("pgx", pgDSN)
	if err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("pg open: %s", err)
	}

	// Apply PG migrations (tenants, api_keys, metering_events).
	if err := goose.SetDialect("postgres"); err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("goose dialect: %s", err)
	}
	// Path is relative to this package directory (internal/logingest), so
	// "../../migrations" resolves to backend/migrations — same as in
	// internal/ingest/cross_tenant_helpers_test.go.
	if err := goose.Up(meteringPGPool, "../../migrations"); err != nil {
		_ = pool.Purge(pg)
		log.Fatalf("goose up: %s", err)
	}

	// ---- CH via chquery/chtest (logs_v1 migration) ----
	meteringCHFix = chtest.StartCH(stderrFatal{}, "20260527120000_create_logs_v1.sql")

	code := m.Run()

	if meteringPGPool != nil {
		_ = meteringPGPool.Close()
	}
	if meteringCHFix != nil {
		_ = meteringCHFix.Close()
	}
	_ = pool.Purge(pg)
	os.Exit(code)
}

// stderrFatal routes chtest.FatalReporter to log.Fatalf for TestMain
// (no *testing.T available at package-init time).
type stderrFatal struct{}

func (stderrFatal) Helper()                   {}
func (stderrFatal) Fatalf(f string, a ...any) { log.Fatalf(f, a...) }

// seedTenantWithKey inserts a tenant + api_key row into PG and returns
// (tenant_id, bearer_plaintext). Unique per call — no cross-test collisions.
func seedTenantWithKey(t *testing.T, db *sql.DB, tenantName string) (uuid.UUID, string) {
	t.Helper()
	tid := uuid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO tenants (id, name, plan) VALUES ($1, $2, 'free')`, tid, tenantName)
	require.NoError(t, err)

	plaintext := "test-log-key-" + tenantName + "-" + tid.String()[:8]
	hashed, err := apikey.Hash(plaintext)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO api_keys (tenant_id, name, hashed_key, scope) VALUES ($1, $2, $3, 'read-write')`,
		tid, tenantName+"-primary", hashed)
	require.NoError(t, err)
	return tid, plaintext
}

// bearerCtx returns a context with the Authorization bearer stuffed into
// collector client.Info.Metadata — the exact shape that ExtractBearer reads.
// This avoids spinning a full OTLP receiver just to test the consumer.
func bearerCtx(bearer string) context.Context {
	md := client.NewMetadata(map[string][]string{
		"authorization": {"Bearer " + bearer},
	})
	return client.NewContext(context.Background(), client.Info{Metadata: md})
}

// fixtureLogs builds a plog.Logs with `n` log records under a single
// resource (service.name = "test-svc").
func fixtureLogs(n int) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "test-svc")
	sl := rl.ScopeLogs().AppendEmpty()
	now := time.Now()
	for i := 0; i < n; i++ {
		rec := sl.LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i) * time.Millisecond)))
		rec.SetObservedTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i)*time.Millisecond + time.Microsecond)))
		rec.SetSeverityText("INFO")
		rec.SetSeverityNumber(plog.SeverityNumberInfo)
		rec.Body().SetStr(fmt.Sprintf("log line %d", i))
	}
	return ld
}

// TestSlice2_MeteringSignalLog is the AC #9 gate: ConsumeLogs must write
// a metering_events row with signal_type='log' and count=N.
func TestSlice2_MeteringSignalLog(t *testing.T) {
	// Wire the in-process consumer with real PG + CH.
	chConn, err := chquery.Connect(context.Background(), meteringCHFix.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = chConn.Close() })

	resolver := auth.NewPGResolver(meteringPGPool)
	reg := prometheus.NewRegistry()
	base := ingestshared.NewBaseMetrics(reg, "log")
	metering := ingestshared.NewMetering(meteringPGPool, base, "log")
	t.Cleanup(metering.Close) // future-proof: today Close is no-op, but registering it now means if Drain ever times out the goroutine is reaped

	consumer := logingest.NewLogConsumer(resolver, chConn, metering, base)

	// Seed a tenant + API key.
	tid, plaintext := seedTenantWithKey(t, meteringPGPool, "acme-log")

	// Build 3 log records and call ConsumeLogs.
	const logCount = 3
	ld := fixtureLogs(logCount)
	ctx := bearerCtx(plaintext)

	require.NoError(t, consumer.ConsumeLogs(ctx, ld),
		"ConsumeLogs must succeed with valid bearer and real CH")

	// Drain blocks until the async metering worker flushes to PG.
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	metering.Drain(drainCtx)

	// Assert: exactly one metering_events row for this tenant with
	// signal_type='log' and count summing to logCount.
	t.Run("metering_events row exists with signal_type=log", func(t *testing.T) {
		var rowCount int
		err := meteringPGPool.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM metering_events WHERE tenant_id=$1 AND signal_type='log'`,
			tid,
		).Scan(&rowCount)
		require.NoError(t, err)
		require.Equal(t, 1, rowCount,
			"expected exactly 1 metering_events row for signal_type='log'")
	})

	t.Run("metering_events count equals log record count", func(t *testing.T) {
		var totalCount int64
		err := meteringPGPool.QueryRowContext(context.Background(),
			`SELECT COALESCE(SUM(count), 0) FROM metering_events WHERE tenant_id=$1 AND signal_type='log'`,
			tid,
		).Scan(&totalCount)
		require.NoError(t, err)
		require.Equal(t, int64(logCount), totalCount,
			"metering count must equal the number of log records sent")
	})

	t.Run("no cross-signal bleed: signal_type=trace count is zero", func(t *testing.T) {
		var traceCount int
		err := meteringPGPool.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM metering_events WHERE tenant_id=$1 AND signal_type='trace'`,
			tid,
		).Scan(&traceCount)
		require.NoError(t, err)
		require.Equal(t, 0, traceCount,
			"log consumer must not write signal_type='trace' rows")
	})
}
