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
