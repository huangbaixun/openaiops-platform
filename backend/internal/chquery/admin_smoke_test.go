//go:build integration

package chquery_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery/chtest"
)

// TestAdminConn_ListTenants_AcrossTenants exercises AdminConn's whitelisted
// AdminListTenants kind against real CH. The Row Policy on traces_v1
// references getSetting('custom_tenant_id') and applies TO the openaiops
// user, so under the default test user the policy filters the result to
// zero rows (AdminConn injects an empty sentinel custom_tenant_id to avoid
// UNKNOWN_SETTING errors — see admin.go).
//
// What this test proves end-to-end:
//   - AdminQuery does NOT panic for missing tenant_id in ctx (the whole
//     point of bypassing MustTenantScope).
//   - The whitelisted SQL is syntactically valid against the real
//     traces_v1 schema.
//   - The driver completes the round trip without error.
//
// What this test deliberately does NOT prove:
//   - That production topo-engine can see all tenants. That depends on
//     the operator-managed CH user / Row Policy override, which is a
//     deploy concern outside T2's scope.
func TestAdminConn_ListTenants_AcrossTenants(t *testing.T) {
	fixture := chtest.StartCH(t,
		"20260525120000_create_traces_v1.sql",
		"20260527120000_create_topology_edges_v1.sql",
	)
	defer fixture.Close()

	conn, err := chquery.Connect(context.Background(), fixture.DSN)
	require.NoError(t, err)
	defer conn.Close()

	tenantA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	tenantB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	seedTraceForTenant(t, conn, tenantA)
	seedTraceForTenant(t, conn, tenantB)

	admin := chquery.NewAdminConn(conn)
	rows, err := admin.AdminQuery(
		context.Background(), // no tenant_id — that's the whole point of AdminConn
		chquery.AdminListTenants,
		time.Now().Add(-1*time.Hour),
	)
	require.NoError(t, err, "AdminQuery must not error — that's T2's core contract")
	defer rows.Close()

	// Drain to ensure the driver doesn't surface an error mid-stream.
	for rows.Next() {
		var tid string
		require.NoError(t, rows.Scan(&tid))
	}
	require.NoError(t, rows.Err())
}

func seedTraceForTenant(t *testing.T, conn *chquery.Conn, tidStr string) {
	t.Helper()
	ctx := auth.WithTenant(context.Background(), uuid.MustParse(tidStr), "seed-"+tidStr)
	batch, err := conn.PrepareBatch(ctx,
		`INSERT INTO traces_v1 (tenant_id, trace_id, span_id, parent_span_id, service, operation,
            ts, duration, status, span_kind, resource_attributes, attributes) VALUES`)
	require.NoError(t, err)
	require.NoError(t, batch.Append(
		tidStr, "trace1", "span1", "",
		"svc", "op", time.Now().UTC(), uint64(1000), "Ok", "Server",
		map[string]string{}, map[string]string{},
	))
	require.NoError(t, batch.Send())
}
