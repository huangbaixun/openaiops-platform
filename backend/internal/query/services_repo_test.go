//go:build integration

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

// seedServiceStats inserts a single row into service_stats_v1 under the
// tenant in ctx. ts_bucket = now-2min so it falls inside any window >= 15m.
func seedServiceStats(t *testing.T, conn *chquery.Conn, ctx context.Context, tidStr, service, kind string, calls, errors, p95 uint64) {
	t.Helper()
	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO service_stats_v1 (tenant_id, ts_bucket, service, span_kind, calls, errors, p95_duration) VALUES`)
	require.NoError(t, err)
	require.NoError(t, batch.Append(tidStr, bucket, service, kind, calls, errors, p95))
	require.NoError(t, batch.Send())
}

// seedTopologyEdge inserts a single row into topology_edges_v1.
func seedTopologyEdge(t *testing.T, conn *chquery.Conn, ctx context.Context, tidStr, caller, callerKind, callee, calleeKind string, calls, errors, p95 uint64) {
	t.Helper()
	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO topology_edges_v1 (tenant_id, ts_bucket, caller_service, caller_kind, callee_service, callee_kind, calls, errors, p95_duration) VALUES`)
	require.NoError(t, err)
	require.NoError(t, batch.Append(tidStr, bucket, caller, callerKind, callee, calleeKind, calls, errors, p95))
	require.NoError(t, batch.Send())
}

// ---- List ------------------------------------------------------------------

func TestServicesRepo_List_TenantScoped(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tidA := uuid.MustParse("a1111111-1111-1111-1111-111111111111")
	tidB := uuid.MustParse("b2222222-2222-2222-2222-222222222222")
	ctxA := auth.WithTenant(context.Background(), tidA, "acme")
	ctxB := auth.WithTenant(context.Background(), tidB, "beta")

	// A: 1 Server (inbound), 1 Client (outbound). B: 1 Server.
	seedServiceStats(t, conn, ctxA, tidA.String(), "svc-a-front", "Server", 10, 1, 500_000_000)
	seedServiceStats(t, conn, ctxA, tidA.String(), "svc-a-front", "Client", 7, 0, 100_000_000)
	seedServiceStats(t, conn, ctxB, tidB.String(), "svc-b-front", "Server", 3, 0, 200_000_000)

	repo := query.NewServicesRepo(conn)

	itemsA, err := repo.List(ctxA, "1h", 10, "calls")
	require.NoError(t, err)
	require.Len(t, itemsA, 1, "A sees its own service")
	require.Equal(t, "svc-a-front", itemsA[0].Service)
	require.Equal(t, uint64(10), itemsA[0].InboundCalls)
	require.Equal(t, uint64(7), itemsA[0].OutboundCalls)
	require.InDelta(t, 0.1, itemsA[0].InboundErrorRate, 1e-9)
	require.InDelta(t, 500.0, itemsA[0].InboundP95Ms, 1e-9)

	itemsB, err := repo.List(ctxB, "1h", 10, "calls")
	require.NoError(t, err)
	require.Len(t, itemsB, 1)
	require.Equal(t, "svc-b-front", itemsB[0].Service)
}

func TestServicesRepo_List_Sorts(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("c3333333-3333-3333-3333-333333333333")
	ctx := auth.WithTenant(context.Background(), tid, "gamma")

	// svc-hi: more calls; svc-err: more errors; svc-p95: higher p95.
	seedServiceStats(t, conn, ctx, tid.String(), "svc-hi", "Server", 100, 1, 50_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "svc-err", "Server", 10, 9, 50_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "svc-p95", "Server", 5, 0, 999_000_000)

	repo := query.NewServicesRepo(conn)

	byCalls, err := repo.List(ctx, "1h", 10, "calls")
	require.NoError(t, err)
	require.Len(t, byCalls, 3)
	require.Equal(t, "svc-hi", byCalls[0].Service, "calls sort first row")

	byErrors, err := repo.List(ctx, "1h", 10, "errors")
	require.NoError(t, err)
	require.Equal(t, "svc-err", byErrors[0].Service, "errors sort first row")

	byP95, err := repo.List(ctx, "1h", 10, "p95")
	require.NoError(t, err)
	require.Equal(t, "svc-p95", byP95[0].Service, "p95 sort first row")
}

func TestServicesRepo_List_Empty(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("d4444444-4444-4444-4444-444444444444")
	ctx := auth.WithTenant(context.Background(), tid, "delta")

	repo := query.NewServicesRepo(conn)
	items, err := repo.List(ctx, "1h", 10, "calls")
	require.NoError(t, err)
	require.NotNil(t, items, "must be empty slice not nil")
	require.Len(t, items, 0)
}

// ---- Detail ----------------------------------------------------------------

func TestServicesRepo_Detail_HappyPath(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("e5555555-5555-5555-5555-555555555555")
	ctx := auth.WithTenant(context.Background(), tid, "epsilon")

	// Self stats.
	seedServiceStats(t, conn, ctx, tid.String(), "checkout", "Server", 20, 2, 300_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "checkout", "Client", 10, 0, 100_000_000)
	// Inbound: gateway -> checkout.
	seedTopologyEdge(t, conn, ctx, tid.String(), "gateway", "service", "checkout", "service", 18, 1, 250_000_000)
	// Outbound: checkout -> redis (external) + checkout -> orders (service).
	seedTopologyEdge(t, conn, ctx, tid.String(), "checkout", "service", "redis", "external", 8, 0, 50_000_000)
	seedTopologyEdge(t, conn, ctx, tid.String(), "checkout", "service", "orders", "service", 2, 0, 80_000_000)

	repo := query.NewServicesRepo(conn)
	resp, err := repo.Detail(ctx, "checkout", "1h")
	require.NoError(t, err)
	require.NotNil(t, resp, "checkout must exist for this tenant")

	require.Equal(t, "checkout", resp.Service)
	require.Equal(t, uint64(20), resp.Stats.Inbound.Calls)
	require.Equal(t, uint64(10), resp.Stats.Outbound.Calls)
	require.InDelta(t, 0.1, resp.Stats.Inbound.ErrorRate, 1e-9)

	require.Len(t, resp.Dependencies.Inbound, 1)
	require.Equal(t, "gateway", resp.Dependencies.Inbound[0].Peer)
	require.Equal(t, "service", resp.Dependencies.Inbound[0].PeerKind)

	require.Len(t, resp.Dependencies.Outbound, 2)
	// Outbound sort by calls DESC: redis (8) before orders (2).
	require.Equal(t, "redis", resp.Dependencies.Outbound[0].Peer)
	require.Equal(t, "external", resp.Dependencies.Outbound[0].PeerKind)
	require.Equal(t, "orders", resp.Dependencies.Outbound[1].Peer)
}

func TestServicesRepo_Detail_Missing_ReturnsNil(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("f6666666-6666-6666-6666-666666666666")
	ctx := auth.WithTenant(context.Background(), tid, "zeta")

	repo := query.NewServicesRepo(conn)
	resp, err := repo.Detail(ctx, "ghost-service", "1h")
	require.NoError(t, err)
	require.Nil(t, resp, "missing service must return nil so handler emits 404")
}

func TestServicesRepo_Detail_CrossTenant_ReturnsNil(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tidA := uuid.MustParse("a7777777-7777-7777-7777-777777777777")
	tidB := uuid.MustParse("b8888888-8888-8888-8888-888888888888")
	ctxA := auth.WithTenant(context.Background(), tidA, "acme")
	ctxB := auth.WithTenant(context.Background(), tidB, "beta")

	// Seed A's checkout.
	seedServiceStats(t, conn, ctxA, tidA.String(), "checkout", "Server", 20, 2, 300_000_000)

	// B asks for checkout — must NOT see A's data.
	repo := query.NewServicesRepo(conn)
	resp, err := repo.Detail(ctxB, "checkout", "1h")
	require.NoError(t, err)
	require.Nil(t, resp, "B asking for A's service must be invisible (return nil for 404)")
}
