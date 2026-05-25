//go:build integration

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

func TestRepo_List_RoundTrip(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ctx := auth.WithTenant(context.Background(), tid, "acme")

	seedSpans(t, conn, ctx, tid, "trace-aaa", 3)

	repo := query.NewTracesRepo(conn)
	now := time.Now().UTC()
	items, hasMore, err := repo.List(ctx, query.ListParams{
		TsFrom: now.Add(-time.Minute),
		TsTo:   now.Add(time.Minute),
		Limit:  10, Sort: "ts", Order: "desc",
	})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Len(t, items, 1)
	require.Equal(t, uint64(3), items[0].SpanCount)
	require.Equal(t, "frontend", items[0].RootService)
}

func TestRepo_List_HasMore(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ctx := auth.WithTenant(context.Background(), tid, "beta")

	// Seed 5 distinct traces so the GROUP BY yields 5 rows.
	for i := 0; i < 5; i++ {
		traceID := "trace-" + string(rune('a'+i)) + string(rune('a'+i)) + string(rune('a'+i))
		seedSpans(t, conn, ctx, tid, traceID, 1)
	}

	repo := query.NewTracesRepo(conn)
	now := time.Now().UTC()
	items, hasMore, err := repo.List(ctx, query.ListParams{
		TsFrom: now.Add(-time.Minute),
		TsTo:   now.Add(time.Minute),
		Limit:  3, Sort: "ts", Order: "desc",
	})
	require.NoError(t, err)
	require.True(t, hasMore, "5 traces with limit=3 must report has_more=true")
	require.Len(t, items, 3)
}
