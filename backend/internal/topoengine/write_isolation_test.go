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

// TestTopoEngine_CannotWriteEdgeAcrossTenant verifies engine writes are
// fenced by the tenant in ctx — running RunOnce under tenant B's context
// cannot populate edges/stats for tenant A, even though tenant A's spans
// exist in CH for the same bucket.
//
// Sequence:
//  1. Seed tidA spans for the bucket.
//  2. Run RunOnce with tidB ctx → assert B's edges/stats are empty
//     (no spans for B) AND A's edges/stats are still empty (B's run
//     must not have touched A's tenant space).
//  3. Run RunOnce with tidA ctx → A's edges/stats now populated,
//     B's edges/stats still empty.
//
// This protects the Row Policy + chquery scoping contract from a future
// engine refactor that accidentally hoists tenant_id into the query body
// instead of the session setting.
func TestTopoEngine_CannotWriteEdgeAcrossTenant(t *testing.T) {
	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	tidAStr := "aaa1aaa1-aaa1-aaa1-aaa1-aaa1aaa1aaa1"
	tidBStr := "bbb1bbb1-bbb1-bbb1-bbb1-bbb1bbb1bbb1"
	tidA := uuid.MustParse(tidAStr)
	tidB := uuid.MustParse(tidBStr)

	ctxA := auth.WithTenant(context.Background(), tidA, "wi-a")
	ctxB := auth.WithTenant(context.Background(), tidB, "wi-b")

	bucket := time.Now().UTC().Truncate(time.Minute).Add(-3 * time.Minute)

	// Seed: only tenant A has spans for this bucket.
	seedSpansForTenant(t, conn, tidAStr, bucket, []SpanSpec{
		{Service: "frontend", SpanID: "wi1", Kind: "Server", Status: "Ok", DurationNs: 1000},
		{Service: "checkout", SpanID: "wi2", ParentSpanID: "wi1", Kind: "Server", Status: "Ok", DurationNs: 2000},
	})

	// Step 1: Run for tenant B (who has no spans).
	require.NoError(t, eng.RunOnce(ctxB, bucket), "RunOnce(ctxB) should not error")

	// B sees nothing under its own scope.
	require.Empty(t, queryEdges(t, conn, ctxB, bucket),
		"after RunOnce(ctxB) with no B spans, B's edges must be empty")
	require.Empty(t, queryStats(t, conn, ctxB, bucket),
		"after RunOnce(ctxB) with no B spans, B's stats must be empty")

	// A's space must be untouched — B's run cannot write into A.
	require.Empty(t, queryEdges(t, conn, ctxA, bucket),
		"B's RunOnce must not have written into A's edge space")
	require.Empty(t, queryStats(t, conn, ctxA, bucket),
		"B's RunOnce must not have written into A's stats space")

	// Step 2: Run for tenant A (who has the spans).
	require.NoError(t, eng.RunOnce(ctxA, bucket), "RunOnce(ctxA) should not error")

	require.NotEmpty(t, queryEdges(t, conn, ctxA, bucket),
		"after RunOnce(ctxA), A's edges must be populated")
	require.NotEmpty(t, queryStats(t, conn, ctxA, bucket),
		"after RunOnce(ctxA), A's stats must be populated")

	// B's space must STILL be empty — A's run cannot leak into B.
	require.Empty(t, queryEdges(t, conn, ctxB, bucket),
		"A's RunOnce must not have leaked into B's edge space")
	require.Empty(t, queryStats(t, conn, ctxB, bucket),
		"A's RunOnce must not have leaked into B's stats space")
}
