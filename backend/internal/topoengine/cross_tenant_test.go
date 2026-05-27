//go:build integration

package topoengine_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

// TestSlice3_CrossTenantTopology is the SLICE-3 AC #10 cross-tenant E2E.
//
// Setup phase (HOISTED — runs ONCE before any sub-test, per SLICE-2 T11 lesson):
//   1. Seed tenant A + tenant B in PG with distinct API keys.
//   2. Seed disjoint span graphs for A (checkout->redis) and B (mobile->orders) in CH.
//   3. Run topoengine.RunOnce for each tenant against the same bucket.
//   4. Stand up the production query router on httptest.Server.
//
// Sub-assertions then exercise the API surface:
//   sub1: GET /v1/topology with keyA returns A's services, excludes B's.
//   sub2: GET /v1/topology with keyB returns B's services, excludes A's.
//   sub3: GET /v1/services/checkout with keyB returns 404 (A's service is invisible).
//   sub4: GET /v1/services with keyB excludes A's services from the list.
//   sub5: empty Bearer on all 3 routes -> 401.
//   sub6: garbage Bearer on all 3 routes -> 401.
//
// EXPECTED RED at T7 commit: sub1-sub4 will fail with 404 because
// /v1/topology, /v1/services, and /v1/services/{name} are not yet registered
// in query/router.go — T9 lands the handlers. sub5+sub6 may also return 404
// instead of 401 since the routes don't exist; that's the same RED bucket.
// The idempotency test and write-isolation test (the non-API ones in this
// package) MUST pass at T7 commit time.
func TestSlice3_CrossTenantTopology(t *testing.T) {
	// ---- HOISTED SETUP -----------------------------------------------
	db := pgEnsureSchema(t)
	defer db.Close()

	tidA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tidB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	keyA := seedAPIKey(t, db, tidA, "acme", "plain-key-a-cross-tenant")
	keyB := seedAPIKey(t, db, tidB, "beta", "plain-key-b-cross-tenant")

	eng, conn := setupEngine(t, topoengine.DefaultConfig())
	defer conn.Close()

	bucket := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)

	// Tenant A graph: checkout -> redis (Client edge from checkout).
	seedSpansForTenant(t, conn, tidA.String(), bucket, []SpanSpec{
		{Service: "checkout", SpanID: "a1", Kind: "Server", Status: "Ok", DurationNs: 1500},
		{Service: "redis", SpanID: "a2", ParentSpanID: "a1", Kind: "Client", Status: "Ok", DurationNs: 500},
	})
	// Tenant B graph: mobile -> orders (Client edge from mobile).
	seedSpansForTenant(t, conn, tidB.String(), bucket, []SpanSpec{
		{Service: "mobile", SpanID: "b1", Kind: "Server", Status: "Ok", DurationNs: 2000},
		{Service: "orders", SpanID: "b2", ParentSpanID: "b1", Kind: "Client", Status: "Ok", DurationNs: 800},
	})

	ctxA := auth.WithTenant(context.Background(), tidA, "acme")
	ctxB := auth.WithTenant(context.Background(), tidB, "beta")
	require.NoError(t, eng.RunOnce(ctxA, bucket), "RunOnce for tenant A")
	require.NoError(t, eng.RunOnce(ctxB, bucket), "RunOnce for tenant B")

	srv := startQueryServer(t, db, conn)
	defer srv.Close()

	// ---- SUB-ASSERTIONS ----------------------------------------------

	t.Run("sub1_topology_keyA_sees_only_A_services", func(t *testing.T) {
		status, body := mustGet(t, srv.URL, "/v1/topology?window=1h", keyA)
		require.Equal(t, 200, status, "expected 200; got %d body=%q", status, body)
		require.Contains(t, body, "checkout", "A should see its own service")
		require.Contains(t, body, "redis", "A should see its own service")
		require.NotContains(t, body, "mobile", "A must not see B's service")
		require.NotContains(t, body, "orders", "A must not see B's service")
	})

	t.Run("sub2_topology_keyB_sees_only_B_services", func(t *testing.T) {
		status, body := mustGet(t, srv.URL, "/v1/topology?window=1h", keyB)
		require.Equal(t, 200, status, "expected 200; got %d body=%q", status, body)
		require.Contains(t, body, "mobile", "B should see its own service")
		require.Contains(t, body, "orders", "B should see its own service")
		require.NotContains(t, body, "checkout", "B must not see A's service")
		require.NotContains(t, body, "redis", "B must not see A's service")
	})

	t.Run("sub3_services_detail_keyB_on_A_service_returns_404", func(t *testing.T) {
		status, _ := mustGet(t, srv.URL, "/v1/services/checkout", keyB)
		require.Equal(t, 404, status, "B asking for A's service must 404, not leak")
	})

	t.Run("sub4_services_list_keyB_excludes_A_services", func(t *testing.T) {
		status, body := mustGet(t, srv.URL, "/v1/services?window=1h", keyB)
		require.Equal(t, 200, status, "expected 200; got %d body=%q", status, body)
		require.NotContains(t, body, "checkout", "list must not leak A's services")
		require.NotContains(t, body, "redis", "list must not leak A's services")
	})

	t.Run("sub5_empty_bearer_401_on_all_routes", func(t *testing.T) {
		for _, p := range []string{"/v1/topology?window=1h", "/v1/services?window=1h", "/v1/services/checkout"} {
			status, body := mustGet(t, srv.URL, p, "")
			require.Equal(t, 401, status,
				"empty bearer on %s expected 401; got %d body=%q",
				p, status, strings.TrimSpace(body))
		}
	})

	t.Run("sub6_garbage_bearer_401_on_all_routes", func(t *testing.T) {
		for _, p := range []string{"/v1/topology?window=1h", "/v1/services?window=1h", "/v1/services/checkout"} {
			status, body := mustGet(t, srv.URL, p, "garbage-not-a-real-key")
			require.Equal(t, 401, status,
				"garbage bearer on %s expected 401; got %d body=%q",
				p, status, strings.TrimSpace(body))
		}
	})
}
