//go:build integration

package topoengine_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

// Catchup discovers tenants from PG then per-tenant replays. With two PG tenants
// — one with traces, one idle — only the trace-bearing tenant gets service_stats,
// proving discovery now comes from PG (not the Row-Policy-blocked AdminConn).
func TestTopoEngine_Discovery_FromPG(t *testing.T) {
	db := pgEnsureSchema(t)
	defer db.Close()

	withTraces := uuid.New()
	idle := uuid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO tenants(id, name) VALUES ($1,'with-traces'), ($2,'idle')`, withTraces, idle)
	require.NoError(t, err)

	cfg := topoengine.DefaultConfig()
	eng, conn := setupEngineWithPG(t, cfg, db)

	bucket := topoengine.ClosedBucketAt(timeNowUTC())
	seedSpansForTenant(t, conn, withTraces.String(), bucket, []SpanSpec{
		{Service: "checkout", SpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 1_000_000},
	})

	require.NoError(t, eng.Catchup(context.Background()))

	assert.NotEmpty(t, queryStats(t, conn, authCtx(withTraces), bucket), "tenant with traces gets service_stats")
	assert.Empty(t, queryStats(t, conn, authCtx(idle), bucket), "idle tenant produces no rows")
}
