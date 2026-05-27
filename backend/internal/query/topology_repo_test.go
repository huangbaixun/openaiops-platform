//go:build integration

package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

func TestTopologyRepo_Topology_TenantScoped(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tidA := uuid.MustParse("aaaa1111-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tidB := uuid.MustParse("bbbb2222-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctxA := auth.WithTenant(context.Background(), tidA, "acme")
	ctxB := auth.WithTenant(context.Background(), tidB, "beta")

	// A: checkout -> redis (external).
	seedTopologyEdge(t, conn, ctxA, tidA.String(), "checkout", "service", "redis", "external", 5, 0, 100_000_000)
	seedServiceStats(t, conn, ctxA, tidA.String(), "checkout", "Server", 10, 1, 200_000_000)
	// B: mobile -> orders (service).
	seedTopologyEdge(t, conn, ctxB, tidB.String(), "mobile", "service", "orders", "service", 3, 0, 50_000_000)
	seedServiceStats(t, conn, ctxB, tidB.String(), "mobile", "Server", 4, 0, 60_000_000)
	seedServiceStats(t, conn, ctxB, tidB.String(), "orders", "Server", 2, 0, 40_000_000)

	repo := query.NewTopologyRepo(conn)

	respA, err := repo.Topology(ctxA, "1h", 100)
	require.NoError(t, err)
	nodeServicesA := nodeNames(respA.Nodes)
	require.Contains(t, nodeServicesA, "checkout")
	require.Contains(t, nodeServicesA, "redis")
	require.NotContains(t, nodeServicesA, "mobile", "A must not see B's nodes")
	require.NotContains(t, nodeServicesA, "orders", "A must not see B's nodes")

	respB, err := repo.Topology(ctxB, "1h", 100)
	require.NoError(t, err)
	nodeServicesB := nodeNames(respB.Nodes)
	require.Contains(t, nodeServicesB, "mobile")
	require.Contains(t, nodeServicesB, "orders")
	require.NotContains(t, nodeServicesB, "checkout", "B must not see A's nodes")
	require.NotContains(t, nodeServicesB, "redis", "B must not see A's nodes")
}

func TestTopologyRepo_Topology_NodeLimitCapsServices(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("cccc3333-cccc-cccc-cccc-cccccccccccc")
	ctx := auth.WithTenant(context.Background(), tid, "gamma")

	// Three service nodes with different call counts; node_limit=2 keeps top 2.
	seedServiceStats(t, conn, ctx, tid.String(), "hot", "Server", 100, 0, 50_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "warm", "Server", 50, 0, 60_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "cold", "Server", 5, 0, 70_000_000)

	repo := query.NewTopologyRepo(conn)
	resp, err := repo.Topology(ctx, "1h", 2)
	require.NoError(t, err)

	names := nodeNames(resp.Nodes)
	require.Contains(t, names, "hot")
	require.Contains(t, names, "warm")
	require.NotContains(t, names, "cold", "node_limit=2 must drop cold (lowest calls)")
}

func TestTopologyRepo_Topology_EdgesIncludeExternal(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("dddd4444-dddd-dddd-dddd-dddddddddddd")
	ctx := auth.WithTenant(context.Background(), tid, "delta")

	seedTopologyEdge(t, conn, ctx, tid.String(), "checkout", "service", "redis", "external", 5, 0, 100_000_000)
	seedServiceStats(t, conn, ctx, tid.String(), "checkout", "Server", 10, 0, 200_000_000)

	repo := query.NewTopologyRepo(conn)
	resp, err := repo.Topology(ctx, "1h", 100)
	require.NoError(t, err)

	require.Len(t, resp.Edges, 1)
	require.Equal(t, "checkout", resp.Edges[0].Caller)
	require.Equal(t, "redis", resp.Edges[0].Callee)
	require.Equal(t, "external", resp.Edges[0].CalleeKind)
	require.Equal(t, uint64(5), resp.Edges[0].Calls)

	// Both checkout (service) and redis (external) must appear as nodes.
	names := nodeNames(resp.Nodes)
	require.Contains(t, names, "checkout")
	require.Contains(t, names, "redis")
}

func TestTopologyRepo_Topology_EmptyTenant(t *testing.T) {
	conn := setupCH(t)
	defer conn.Close()

	tid := uuid.MustParse("eeee5555-eeee-eeee-eeee-eeeeeeeeeeee")
	ctx := auth.WithTenant(context.Background(), tid, "epsilon")

	repo := query.NewTopologyRepo(conn)
	resp, err := repo.Topology(ctx, "1h", 100)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Nodes, "must be empty slice not nil")
	require.NotNil(t, resp.Edges, "must be empty slice not nil")
	require.Len(t, resp.Nodes, 0)
	require.Len(t, resp.Edges, 0)
}

// nodeNames extracts the Service field of every node for assertion convenience.
func nodeNames(nodes []query.TopologyNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Service)
	}
	return out
}
