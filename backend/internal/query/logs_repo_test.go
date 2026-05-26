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

func TestLogsRepo_List_TenantScoped(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tidA := uuid.MustParse("aa111111-1111-1111-1111-111111111111")
	tidB := uuid.MustParse("bb222222-2222-2222-2222-222222222222")

	ctxA := auth.WithTenant(context.Background(), tidA, "acme")
	ctxB := auth.WithTenant(context.Background(), tidB, "beta")

	// Insert 3 rows for A (2 with trace "abc", 1 without), 1 row for B.
	seedLog(t, conn, ctxA, tidA.String(), "svc-a", "ERROR", "boom1", "abc")
	seedLog(t, conn, ctxA, tidA.String(), "svc-a", "WARN", "warn1", "abc")
	seedLog(t, conn, ctxA, tidA.String(), "svc-b", "INFO", "info1", "")
	seedLog(t, conn, ctxB, tidB.String(), "svc-b", "ERROR", "boom-b", "")

	repo := query.NewLogsRepo(conn)
	now := time.Now().UTC()
	tsFrom := now.Add(-1 * time.Hour)
	tsTo := now.Add(1 * time.Second)

	// Tenant A sees its own 3 rows.
	gotA, hasMore, err := repo.List(ctxA, query.LogsListParams{
		TsFrom: tsFrom,
		TsTo:   tsTo,
		Limit:  10,
	})
	require.NoError(t, err)
	require.False(t, hasMore, "3 rows with limit=10 must not report has_more")
	require.Len(t, gotA, 3, "tenant A must see exactly 3 rows")

	// Tenant B sees only its own 1 row.
	gotB, _, err := repo.List(ctxB, query.LogsListParams{
		TsFrom: tsFrom,
		TsTo:   tsTo,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, gotB, 1, "tenant B must see exactly 1 row (own)")

	// trace_id filter: tenant A with trace_id="abc" → 2 rows.
	gotByTrace, _, err := repo.List(ctxA, query.LogsListParams{
		TsFrom:  tsFrom,
		TsTo:    tsTo,
		TraceID: "abc",
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, gotByTrace, 2, "trace_id=abc must return 2 rows for tenant A")
}

func TestLogsRepo_List_HasMore(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("cc333333-3333-3333-3333-333333333333")
	ctx := auth.WithTenant(context.Background(), tid, "gamma")

	// Insert 5 rows; query with limit=3 → has_more=true.
	for i := 0; i < 5; i++ {
		seedLog(t, conn, ctx, tid.String(), "svc", "INFO", "msg", "")
	}

	repo := query.NewLogsRepo(conn)
	now := time.Now().UTC()

	items, hasMore, err := repo.List(ctx, query.LogsListParams{
		TsFrom: now.Add(-time.Minute),
		TsTo:   now.Add(time.Second),
		Limit:  3,
	})
	require.NoError(t, err)
	require.True(t, hasMore, "5 rows with limit=3 must report has_more=true")
	require.Len(t, items, 3)
}

func TestLogsRepo_List_SeverityFilter(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("dd444444-4444-4444-4444-444444444444")
	ctx := auth.WithTenant(context.Background(), tid, "delta")

	seedLog(t, conn, ctx, tid.String(), "svc", "ERROR", "err-msg", "")
	seedLog(t, conn, ctx, tid.String(), "svc", "INFO", "info-msg", "")

	repo := query.NewLogsRepo(conn)
	now := time.Now().UTC()

	got, _, err := repo.List(ctx, query.LogsListParams{
		TsFrom:   now.Add(-time.Minute),
		TsTo:     now.Add(time.Second),
		Severity: []string{"ERROR"},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, got, 1, "severity=ERROR filter must return 1 row")
	require.Equal(t, "ERROR", got[0].SeverityText)
}
