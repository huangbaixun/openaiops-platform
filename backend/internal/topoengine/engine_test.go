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

// TestTopoEngine_RunOnce_WritesEdges asserts Pass A produces exactly 3 edges
// from a 4-span trace:
//   - frontend -> checkout (internal, callee_kind=service)
//   - checkout -> payment  (internal, callee_kind=service)
//   - checkout -> redis    (external, callee_kind=external, from db.system attr
//     on a Client span whose parent is in the SAME service — covering the
//     "lost parent / same-service parent" external edge branch).
//
// The redis assertion is the canary for the join_use_nulls=1 setting: without
// it, the LEFT JOIN unmatched right-side LowCardinality columns return ''
// (not NULL) and `a.service IS NULL` would silently never fire, dropping
// every external edge from this path.
func TestTopoEngine_RunOnce_WritesEdges(t *testing.T) {
	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	tidStr := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	tid := uuid.MustParse(tidStr)
	tctx := auth.WithTenant(context.Background(), tid, "test-acme")

	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	seedSpansForTenant(t, conn, tidStr, bucket, []SpanSpec{
		{Service: "frontend", SpanID: "s1", ParentSpanID: "", Kind: "Server", Status: "Ok", DurationNs: 1000},
		{Service: "checkout", SpanID: "s2", ParentSpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 2000},
		{Service: "payment", SpanID: "s3", ParentSpanID: "s2", Kind: "Server", Status: "Ok", DurationNs: 3000},
		{Service: "checkout", SpanID: "s4", ParentSpanID: "s2", Kind: "Client", Status: "Ok", DurationNs: 500,
			Attrs: map[string]string{"db.system": "redis"}},
	})

	require.NoError(t, eng.RunOnce(tctx, bucket))

	edges := queryEdges(t, conn, tctx, bucket)
	require.Len(t, edges, 3, "expected 3 edges, got %+v", edges)

	// caller|callee -> expected callee_kind
	want := map[string]string{
		"frontend|checkout": "service",
		"checkout|payment":  "service",
		"checkout|redis":    "external",
	}
	for _, e := range edges {
		key := e.Caller + "|" + e.Callee
		wantKind, ok := want[key]
		require.True(t, ok, "unexpected edge %s", key)
		require.Equal(t, wantKind, e.CalleeKind, "edge %s: want kind %q, got %q", key, wantKind, e.CalleeKind)
		require.Equal(t, "service", e.CallerKind)
		require.Equal(t, uint64(1), e.Calls)
		require.Equal(t, uint64(0), e.Errors)
	}
}

// TestTopoEngine_RunOnce_WritesServiceStats asserts Pass B produces per-(service, kind)
// rows from a seeded trace shape:
//   - frontend / Server / calls=1 / errors=0
//   - checkout / Server / calls=1 / errors=0
//   - checkout / Client / calls=1 / errors=1 (Error status)
func TestTopoEngine_RunOnce_WritesServiceStats(t *testing.T) {
	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	tidStr := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	tid := uuid.MustParse(tidStr)
	tctx := auth.WithTenant(context.Background(), tid, "test-c")

	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	seedSpansForTenant(t, conn, tidStr, bucket, []SpanSpec{
		{Service: "frontend", SpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 1000},
		{Service: "checkout", SpanID: "s2", ParentSpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 2000},
		{Service: "checkout", SpanID: "s3", ParentSpanID: "s2", Kind: "Client", Status: "Error", DurationNs: 500,
			Attrs: map[string]string{"db.system": "redis"}},
	})

	require.NoError(t, eng.RunOnce(tctx, bucket))

	stats := queryStats(t, conn, tctx, bucket)
	require.Len(t, stats, 3, "expected 3 stats rows, got %+v", stats)

	type key struct{ svc, kind string }
	got := map[key]statsRow{}
	for _, r := range stats {
		got[key{r.Service, r.SpanKind}] = r
	}
	require.Equal(t, uint64(1), got[key{"frontend", "Server"}].Calls)
	require.Equal(t, uint64(0), got[key{"frontend", "Server"}].Errors)
	require.Equal(t, uint64(1), got[key{"checkout", "Server"}].Calls)
	require.Equal(t, uint64(0), got[key{"checkout", "Server"}].Errors)
	require.Equal(t, uint64(1), got[key{"checkout", "Client"}].Calls)
	require.Equal(t, uint64(1), got[key{"checkout", "Client"}].Errors)
}

// TestTopoEngine_Idempotency_DoubleRun asserts ReplacingMergeTree FINAL
// dedupes a same-bucket re-run: edges queried after RunOnce twice equals
// edges queried after RunOnce once. Re-running the same bucket is a no-op
// from the consumer's perspective — this is the contract Catchup relies on.
func TestTopoEngine_Idempotency_DoubleRun(t *testing.T) {
	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	tidStr := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	tid := uuid.MustParse(tidStr)
	tctx := auth.WithTenant(context.Background(), tid, "test-e")
	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)

	seedSpansForTenant(t, conn, tidStr, bucket, []SpanSpec{
		{Service: "frontend", SpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 1000},
		{Service: "checkout", SpanID: "s2", ParentSpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 2000},
	})

	require.NoError(t, eng.RunOnce(tctx, bucket))
	first := queryEdges(t, conn, tctx, bucket)
	require.NoError(t, eng.RunOnce(tctx, bucket))
	second := queryEdges(t, conn, tctx, bucket)
	require.ElementsMatch(t, first, second,
		"ReplacingMergeTree FINAL should dedupe re-runs of the same bucket")
}
