//go:build integration

package topoengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

// TestTopoEngine_CatchupTenant_BackfillsThreeBuckets seeds three consecutive
// 1-minute buckets of spans for a single tenant, calls CatchupTenant(), and
// asserts each bucket produces 1 edge in topology_edges_v1. The second
// CatchupTenant call must be a no-op for those seeded buckets (idempotent
// via FINAL dedup on the ReplacingMergeTree) — verifies replay safety.
//
// Uses CatchupTenant (per-tenant entry point) directly — no PG fixture needed for this single-tenant backfill assertion.
func TestTopoEngine_CatchupTenant_BackfillsThreeBuckets(t *testing.T) {
	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	tidStr := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	tid := uuid.MustParse(tidStr)
	tctx := auth.WithTenant(context.Background(), tid, "test-d")

	b3 := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	b2 := b3.Add(-1 * time.Minute)
	b1 := b3.Add(-2 * time.Minute)

	for _, b := range []time.Time{b1, b2, b3} {
		seedSpansForTenant(t, conn, tidStr, b, []SpanSpec{
			{Service: "frontend", SpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 1000},
			{Service: "checkout", SpanID: "s2", ParentSpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 2000},
		})
	}

	require.NoError(t, eng.CatchupTenant(context.Background(), tid))

	for _, b := range []time.Time{b1, b2, b3} {
		edges := queryEdges(t, conn, tctx, b)
		require.Len(t, edges, 1, "expected 1 edge in bucket %s, got %+v", b, edges)
	}

	// Second CatchupTenant — bucket window is the same so it re-aggregates,
	// but FINAL dedup on topology_edges_v1 (ReplacingMergeTree) collapses each
	// (tenant_id, ts_bucket, caller, callee, callee_kind) tuple to one row.
	require.NoError(t, eng.CatchupTenant(context.Background(), tid))
	for _, b := range []time.Time{b1, b2, b3} {
		edges := queryEdges(t, conn, tctx, b)
		require.Len(t, edges, 1, "post-second-catchup: bucket %s still 1 edge, got %+v", b, edges)
	}
}
