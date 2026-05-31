# PLATFORM-TOPO-1 — topo-engine PG-driven tenant discovery (fix D6) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use harness:subagent-driven-development (recommended) or harness:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/specs/2026-05-31-platform-topo-1-pg-tenant-discovery-design.md` (feature `PLATFORM-TOPO-1`). **ADR:** `docs/decisions/0005-topo-engine-pg-tenant-discovery.md`.

**Goal:** topo-engine discovers the tenant set from the PG `tenants` table (via its already-open `Deps.PG`) instead of the `AdminConn` sentinel path that the `tenant_isolation_*` Row Policy filters to zero rows — fixing D6 (empty `service_stats_v1`) without touching any Row Policy, CH user, or deploy config.

**Architecture:** Two broken admin queries move off `AdminConn`: `activeTenants` → PG `SELECT id FROM tenants`; `lastCompletedBucket` → tenant-scoped `chquery.Conn.QueryRow` (it already carries `tenant_id = ?`). With both gone, `chquery.AdminConn` + sentinel + its lint exception are deleted. The per-tenant aggregation (`edges.go`/`services.go`) is unchanged. Net effect: topo-engine becomes a pure in-isolation-model consumer and the only Row-Policy-bypass mechanism in the codebase is removed.

**Tech Stack:** Go 1.25 (database/sql + pgx stdlib, google/uuid, clickhouse-go via chquery), dockertest (CH + PG fixtures), goose.

---

## Acceptance-criteria → task traceability

| AC | Criterion (abridged) | Task(s) |
|---|---|---|
| 1 | activeTenants sources tenants from PG via Deps.PG; AdminConn discovery gone | T2 |
| 2 | lastCompletedBucket is a tenant-scoped CH query (passes Row Policy, real max bucket) | T1 |
| 3 | chquery.AdminConn + sentinel + lint exception removed; builds; make lint-ch passes | T3 |
| 4 | Fresh stack: after seed + one tick, service_stats/topology_edges populate; /api/v1/services returns data | T4 |
| 5 | shell.spec ⌘K e2e passes; full Playwright green; D6 closed | T4 |
| 6 | No Row Policy DDL / CH user / deploy change; ADR-0005 (done) | all (none touch those); T3 (lint only) |

No orphan criteria. No task touches `out_of_scope` (no exempt CH user, no idle-tenant optimization, no aggregation SQL change).

---

## File structure

**Modified:**
- `backend/internal/topoengine/state.go` — `lastCompletedBucket` → `chquery.Conn.QueryRow`.
- `backend/internal/topoengine/tenants.go` — `activeTenants` → PG; signature drops `since`.
- `backend/internal/topoengine/engine.go` — callers drop the time arg; comments updated.
- `backend/internal/topoengine/types.go` — `Deps.Admin` removed; `PG` comment updated; package doc updated.
- `backend/cmd/topo-engine/main.go` — stop constructing AdminConn; drop `Admin` from Deps; remove operator note.
- `backend/internal/topoengine/test_helpers_test.go` — `setupEngine` drops Admin; add `setupEngineWithPG`.
- `backend/internal/topoengine/catchup_test.go` — drop AdminConn comments (still uses `CatchupTenant`).
- `deploy/lint-no-bare-ch.sh` — remove Rule 2 (AdminConn confinement).

**Deleted:**
- `backend/internal/chquery/admin.go`, `backend/internal/chquery/admin_test.go`, `backend/internal/chquery/admin_smoke_test.go`.

**Untouched:** `edges.go`/`services.go` (aggregation SQL), all `ch-migrations/`, Row Policy DDL, `deploy/docker-compose.yml` CH user, `clickhouse-custom-settings.xml`.

---

## Task 1: `lastCompletedBucket` → tenant-scoped Conn

AC#2.

**Files:**
- Modify: `backend/internal/topoengine/state.go`

- [ ] **Step 1: Confirm `Conn.QueryRow` applies tenant scoping**

Read `backend/internal/chquery/conn.go` around `func (cn *Conn) QueryRow` (line ~81). Confirm it calls `MustTenantScope(ctx, query, args...)` and injects `tenantSettings(tid)` exactly like `Query`/`Exec` (so `tenant_id = ?` gets the ctx tenant and `custom_tenant_id` is set). If it does NOT (unlikely — the package is consistent), use `Conn.Query` + single-row iteration instead and note it. Assume it does for the code below.

- [ ] **Step 2: Rewrite `lastCompletedBucket`**

Replace the body so it reads via the tenant-scoped `Conn` (the ctx already carries the tenant — `CatchupTenant` sets it via `auth.WithTenant`). `MustTenantScope` (inside `QueryRow`) panics if the tenant is absent, preserving the previous guard's intent.

```go
package topoengine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

// lastCompletedBucket returns max(ts_bucket) in topology_edges_v1 for the
// tenant in ctx, or zero time if none. Used by Catchup to decide replay start.
// Runs as a tenant-scoped chquery.Conn query (PLATFORM-TOPO-1 / ADR-0005) — the
// `tenant_id = ?` placeholder + custom_tenant_id are injected by MustTenantScope,
// so the tenant_isolation Row Policy passes for the real tenant.
func (e *Engine) lastCompletedBucket(ctx context.Context) time.Time {
	row := e.deps.CH.QueryRow(ctx,
		`SELECT max(ts_bucket) FROM topology_edges_v1 FINAL WHERE tenant_id = ?`)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		// io.EOF / empty = "no buckets yet" (legitimate zero). Other errors also
		// collapse to zero for best-effort catchup; real CH outages surface via
		// the tick-failure metric at the tick layer.
		if !errors.Is(err, io.EOF) {
			slog.Warn("topoengine: lastCompletedBucket scan error (treating as zero)", "err", err)
		}
		return time.Time{}
	}
	return t
}
```

Note: this drops the `auth` and `chquery` imports from `state.go` (the explicit `auth.TenantID` guard and `chquery.AdminMaxBucket` are gone). Remove them from the import block (shown above — only `context`, `errors`, `io`, `log/slog`, `time` remain).

- [ ] **Step 3: Build + run topoengine integration (catchup test exercises this path)**

Run: `cd backend && go build ./... && go test -tags=integration -timeout 300s -run TestTopoEngine_CatchupTenant ./internal/topoengine/`
Expected: PASS. `CatchupTenant` calls `lastCompletedBucket` with a tenant ctx; the chtest CH user is Row-Policy-bound, and now the query passes because it carries the real `tenant_id`/`custom_tenant_id` (previously the AdminConn sentinel would have returned 0 here too — the test still passed because first-boot zero and seeded zero both hit the "replay full window" branch; it will still pass, now reading the true max on the idempotent second run). If it fails, inspect whether `QueryRow` applied scoping (Step 1).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/topoengine/state.go
git commit -m "feat(topo-1): lastCompletedBucket via tenant-scoped Conn (drop AdminMaxBucket)"
```

---

## Task 2: `activeTenants` → PG

AC#1.

**Files:**
- Modify: `backend/internal/topoengine/tenants.go`
- Modify: `backend/internal/topoengine/engine.go`
- Modify: `backend/internal/topoengine/types.go` (PG comment + package doc; keep `Admin` field for now — removed in T3)
- Modify: `backend/internal/topoengine/test_helpers_test.go` (add `setupEngineWithPG`)
- Create test: append to `backend/internal/topoengine/tenants_test.go` (new file)

- [ ] **Step 1: Rewrite `activeTenants` to read PG**

Replace `tenants.go` entirely:

```go
package topoengine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// activeTenants returns every registered tenant id from the PG tenants table
// (the authoritative registry) via the engine's PG connection. PLATFORM-TOPO-1
// / ADR-0005 replaced the chquery.AdminConn discovery (SELECT DISTINCT tenant_id
// FROM traces_v1), which the tenant_isolation Row Policy filtered to zero rows
// (D6). Idle tenants are included and aggregate to zero rows — acceptable at the
// current scale (a future optimization may pre-filter to tenants with traces in
// the window).
func (e *Engine) activeTenants(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := e.deps.PG.QueryContext(ctx, `SELECT id FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("topoengine: list tenants from pg: %w", err)
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var tid uuid.UUID
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("topoengine: scan tenant id: %w", err)
		}
		out = append(out, tid)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Update callers in `engine.go`**

`RunBucket` calls `e.activeTenants(adminCtx, bucket)` → `e.activeTenants(adminCtx)`. `Catchup` calls `e.activeTenants(adminCtx, since)` → `e.activeTenants(adminCtx)`, and the now-unused `since := time.Now()...Add(-e.cfg.CatchupMax)` line in `Catchup` is removed (CatchupMax is still used inside `CatchupTenant`, so leave that). Update the two doc comments that say "discovery uses chquery.AdminConn / sentinel ... operator-managed CH user exempted" to: "discovery reads the PG tenants table (ADR-0005)." Keep the `adminCtx` param name OR rename to `ctx` — your call; if renaming, update both signatures and the bodies consistently. Verify `time` is still imported in engine.go (it is, used elsewhere).

- [ ] **Step 3: Update `types.go` docs (keep Admin field for now)**

In `types.go`: change the `PG` field comment from `// reserved for future idempotency state` to `// tenant discovery: SELECT id FROM tenants (PLATFORM-TOPO-1)`. Update the package doc paragraph "Tenant trust: discovery uses chquery.AdminConn (whitelisted SQL only)." to "Tenant trust: discovery reads the PG tenants table; per-tenant aggregation uses chquery.Conn under auth.WithTenant(...) (SQL filter + Row Policy + custom_tenant_id)." Leave the `Admin *chquery.AdminConn` field in place — Task 3 removes it (keeps this task's diff focused and the tree compiling).

- [ ] **Step 4: Add a PG-enabled engine helper**

In `test_helpers_test.go`, add below `setupEngine`:

```go
// setupEngineWithPG wires an engine whose Deps.PG points at the shared ephemeral
// Postgres, so activeTenants (PG-driven, PLATFORM-TOPO-1) can be exercised.
func setupEngineWithPG(t *testing.T, cfg topoengine.Config, db *sql.DB) (*topoengine.Engine, *chquery.Conn) {
	t.Helper()
	conn := setupCH(t)
	reg := prometheus.NewRegistry()
	metrics := topoengine.NewMetrics(reg)
	eng := topoengine.New(cfg, topoengine.Deps{CH: conn, PG: db}, metrics)
	return eng, conn
}
```

(Admin is omitted — nil — which is fine: no source path reads `deps.Admin` after Tasks 1–2.)

- [ ] **Step 5: Write the failing integration test**

Create `backend/internal/topoengine/tenants_test.go`:

```go
//go:build integration

package topoengine_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/topoengine"
)

// activeTenants is unexported; exercise it through Catchup, which lists tenants
// then per-tenant replays. With two PG tenants — one with traces, one idle — the
// tenant that has spans must get service_stats rows and the idle one must not,
// proving discovery now comes from PG (not the Row-Policy-blocked AdminConn).
func TestTopoEngine_Discovery_FromPG(t *testing.T) {
	db := pgEnsureSchema(t) // truncates tenants+api_keys
	defer db.Close()

	withTraces := uuid.New()
	idle := uuid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO tenants(id, name) VALUES ($1,'with-traces'), ($2,'idle')`, withTraces, idle)
	require.NoError(t, err)

	cfg := topoengine.DefaultConfig()
	eng, conn := setupEngineWithPG(t, cfg, db)

	bucket := topoengine.ClosedBucketAt(timeNowUTC())
	seedSpansForTenant(t, conn, withTraces.String(), bucket, []SpanSpec{
		{Service: "checkout", SpanID: "s1", Kind: "Server", Status: "Ok", DurationNs: 1_000_000},
	})

	// Catchup discovers tenants from PG and replays each.
	require.NoError(t, eng.Catchup(context.Background()))

	ctxWith := authCtx(withTraces)
	ctxIdle := authCtx(idle)
	assert.NotEmpty(t, queryStats(t, conn, ctxWith, bucket), "tenant with traces gets service_stats")
	assert.Empty(t, queryStats(t, conn, ctxIdle, bucket), "idle tenant produces no rows")
}
```

This test needs two tiny helpers. Add them to `test_helpers_test.go` if absent:

```go
import "time" // already imported

func timeNowUTC() time.Time { return time.Now().UTC() }

// authCtx builds a tenant-scoped context for queryStats/queryEdges.
func authCtx(tid uuid.UUID) context.Context {
	return auth.WithTenant(context.Background(), tid, "test")
}
```

(If `auth` / `context` / `time` are already imported in `test_helpers_test.go` — they are — don't duplicate.)

- [ ] **Step 6: Run it — RED first (if you wrote the test before the impl) then GREEN**

Run: `cd backend && go test -tags=integration -timeout 300s -run TestTopoEngine_Discovery_FromPG ./internal/topoengine/`
Expected: PASS — `with-traces` has stats, `idle` does not. (Docker required; if down → BLOCKED.)

- [ ] **Step 7: Full topoengine integration + build**

Run: `cd backend && go build ./... && go test -tags=integration -timeout 360s ./internal/topoengine/`
Expected: all PASS (existing bucket/catchup/cross-tenant/write-isolation tests + the new discovery test). The existing tests that called `CatchupTenant` directly still pass; they may now also pass via `Catchup` discovery, but don't change them here.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/topoengine/tenants.go backend/internal/topoengine/engine.go backend/internal/topoengine/types.go backend/internal/topoengine/test_helpers_test.go backend/internal/topoengine/tenants_test.go
git commit -m "feat(topo-1): activeTenants reads PG tenants table (fix D6 discovery)"
```

---

## Task 3: Remove `chquery.AdminConn` + lint exception

AC#3, AC#6 (lint).

**Files:**
- Delete: `backend/internal/chquery/admin.go`, `backend/internal/chquery/admin_test.go`, `backend/internal/chquery/admin_smoke_test.go`
- Modify: `backend/internal/topoengine/types.go` (remove `Admin` field)
- Modify: `backend/cmd/topo-engine/main.go` (remove AdminConn construction + operator note)
- Modify: `backend/internal/topoengine/test_helpers_test.go` (drop Admin from `setupEngine`)
- Modify: `backend/internal/topoengine/catchup_test.go` (drop AdminConn comments)
- Modify: `deploy/lint-no-bare-ch.sh` (remove Rule 2)

- [ ] **Step 1: Confirm AdminConn has no remaining source users**

Run: `cd backend && grep -rn "AdminConn\|NewAdminConn\|AdminQuery\|AdminListTenants\|AdminMaxBucket\|chquery.Admin\|deps.Admin" --include=*.go .`
Expected references only in: `chquery/admin*.go` (to delete), `topoengine/types.go` (the `Admin` field), `cmd/topo-engine/main.go` (construction), `topoengine/test_helpers_test.go` (`setupEngine` builds it), `topoengine/catchup_test.go` (comments only). If any *source* (non-test, non-admin.go) path still calls an Admin method, STOP — Tasks 1/2 missed a caller; fix that first.

- [ ] **Step 2: Delete the AdminConn files**

```bash
git rm backend/internal/chquery/admin.go backend/internal/chquery/admin_test.go backend/internal/chquery/admin_smoke_test.go
```

- [ ] **Step 3: Remove `Admin` from `Deps`**

In `types.go`, delete the line `Admin *chquery.AdminConn // tenant-unaware admin queries` from the `Deps` struct. If `chquery` is now unused in `types.go`, keep it only if `CH *chquery.Conn` still references it (it does) — leave the import.

- [ ] **Step 4: Update `cmd/topo-engine/main.go`**

Remove the operator-note comment block (the lines about "Operator note (T2 known_drift): activeTenants() runs under AdminConn ... CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 ... See backend/internal/chquery/admin.go godoc."). Remove `admin := chquery.NewAdminConn(ch)`. Change `topoengine.Deps{CH: ch, Admin: admin, PG: db}` → `topoengine.Deps{CH: ch, PG: db}`. Add a one-line comment where helpful: `// Tenant discovery reads the PG tenants table (ADR-0005); no Row-Policy-exempt CH user needed.` Verify `chquery` is still imported (used by `chquery.Connect`).

- [ ] **Step 5: Update `test_helpers_test.go` `setupEngine`**

Change:
```go
func setupEngine(t *testing.T, cfg topoengine.Config) (*topoengine.Engine, *chquery.Conn) {
	t.Helper()
	conn := setupCH(t)
	reg := prometheus.NewRegistry()
	metrics := topoengine.NewMetrics(reg)
	eng := topoengine.New(cfg, topoengine.Deps{CH: conn, PG: nil}, metrics)
	return eng, conn
}
```
(Drop the `admin := chquery.NewAdminConn(conn)` line and the `Admin: admin` field.) Tests that use `setupEngine` (bucket_test, engine_test, write_isolation_test, cross_tenant_test) call only `RunOnce`/`runBucketForTenant`/`CatchupTenant`-style entry points that don't need discovery, so they keep working with `PG: nil`. If any of them call `RunBucket`/`Catchup` (which now need PG), switch that test to `setupEngineWithPG` with a `pgEnsureSchema(t)` db and report which.

- [ ] **Step 6: Clean `catchup_test.go` comments**

Remove the comment paragraph referencing "AdminConn's empty [sentinel] ... the chtest fixture's openaiops user is bound by the tenant_isolation Row Policy" (it explained why the test used `CatchupTenant` over `Catchup`). The test still legitimately calls `CatchupTenant` (per-tenant entry point); just drop the now-obsolete AdminConn rationale, replacing with: `// Uses CatchupTenant (per-tenant entry point) directly — no PG fixture needed for this single-tenant backfill assertion.`

- [ ] **Step 7: Remove Rule 2 from `deploy/lint-no-bare-ch.sh`**

Open `deploy/lint-no-bare-ch.sh`. Delete the "Rule 2: chquery.AdminConn may only be constructed under topo-engine" block — the comment, the `BAD_ADMIN=$(grep ... NewAdminConn ...)` command, and its `if [ -n "$BAD_ADMIN" ]; then ... fi` failure branch. Update the final success `echo` to drop "; AdminConn confined to topo-engine subsystem" so it reads e.g. `echo "lint-no-bare-ch: OK ($SCAN_DIRS clean)"`. Leave Rule 1 (bare `ch.Query`/`ch.Exec` ban in internal/query + internal/ingest) intact.

- [ ] **Step 8: Build + lint + full backend tests**

```bash
cd backend && go build ./...
bash ../deploy/lint-no-bare-ch.sh        # or: make lint-ch (from repo root)
go test ./...                            # unit
go test -tags=integration -timeout 360s ./internal/chquery/ ./internal/topoengine/ ./internal/auth/ ./internal/identity/
```
Expected: build clean; lint prints OK; unit green; integration green. The deleted `admin_test.go`/`admin_smoke_test.go` no longer run; nothing else references AdminConn.

- [ ] **Step 9: Commit**

```bash
git add -A backend/internal/chquery/ backend/internal/topoengine/ backend/cmd/topo-engine/main.go deploy/lint-no-bare-ch.sh
git commit -m "refactor(topo-1): remove chquery.AdminConn + sentinel + lint exception"
```

---

## Task 4: D6 end-to-end verification + close drift

AC#4, AC#5.

**Files:** none (verification + docs).

- [ ] **Step 1: Rebuild the topo-engine image (LESSON: `make up` does not --build)**

```bash
cd /Users/huangbaixun/code_space/openaiops-platform
docker-compose -f deploy/docker-compose.yml build topo-engine
docker-compose -f deploy/docker-compose.yml up -d --no-deps topo-engine
make seed
```

- [ ] **Step 2: Seed trace + topology data (ingester host port is 14317 on this box, not 4317)**

```bash
cd backend && go run ./cmd/seed-traces -target localhost:14317 && go run ./cmd/seed-topology -target localhost:14317
```

- [ ] **Step 3: Wait one+ tick, then confirm topo-engine populated service_stats**

Poll up to ~200s (topo-engine processes the closed minute bucket on a ~1m tick + lag):

```bash
for i in $(seq 1 20); do
  body=$(curl -sk "https://localhost/api/v1/services?window=1h" -H "Authorization: Bearer test-key-acme")
  echo "$body" | grep -q '"service"' && { echo "POPULATED: $body"; break; }
  sleep 10
done
```
Expected: a non-empty `items` array containing services (e.g. checkout/payment). Also check the engine isn't erroring: `docker logs deploy-topo-engine-1 2>&1 | tail -15` (no repeated tick failures; tenants discovered > 0). If still empty after 200s, inspect the logs — topo-engine should now log/serve a non-zero tenant count from PG.

- [ ] **Step 4: Run the ⌘K e2e (previously red on D6) + the full Playwright suite**

```bash
cd frontend && npx playwright test shell.spec.ts           # ⌘K must now pass
cd frontend && npx playwright test                          # full suite
```
Expected: `shell.spec` 3/3 (incl. ⌘K → jumps to a service); full suite green (34/34 — the previously-red ⌘K resolved, no new failures). If the cross-tenant isolation specs or any prior-green spec regress, investigate (must not happen — no policy/data-path change).

- [ ] **Step 5: Close D6 in progress.json**

Edit `docs/claude-progress.json`: change the `D6` known_drift entry's `severity` to `"resolved"` and prepend `RESOLVED 2026-05-31 (PLATFORM-TOPO-1): ` to its `item`, summarizing the fix (topo-engine now discovers tenants from PG; AdminConn removed; ⌘K e2e green; no Row Policy/CH-user change). Leave the historical detail intact.

- [ ] **Step 6: Commit**

```bash
git add docs/claude-progress.json
git commit -m "test(topo-1): verify D6 fixed e2e (services populate, ⌘K green); close drift D6"
```

- [ ] **Step 7: Hand off to verification-before-completion → finishing-a-development-branch.**

---

## Self-review notes

- **Spec coverage:** lastCompletedBucket→T1; activeTenants→T2; AdminConn+lint removal→T3; D6 e2e + close→T4; ADR-0005 already written; no Row Policy/CH-user/deploy change in any task. All 6 ACs traced.
- **Placeholder scan:** no TBD/"handle errors"/"similar to". Each code step shows the full function or the exact edit. The only conditional instructions ("if QueryRow doesn't scope…", "if a test calls RunBucket…") are explicit fallbacks with concrete actions, not placeholders.
- **Type consistency:** `activeTenants(ctx) ([]uuid.UUID, error)` — new signature used by both `RunBucket` and `Catchup` (T2). `lastCompletedBucket(ctx) time.Time` unchanged signature, new body (T1). `Deps{CH, PG}` (Admin removed) consistent across `New` callers: `setupEngine`/`setupEngineWithPG` (tests) + `cmd/topo-engine/main.go` (T2 adds PG helper, T3 removes Admin everywhere). `e.deps.PG.QueryContext` / `e.deps.CH.QueryRow` match the real `*sql.DB` / `*chquery.Conn` method sets.
- **Out-of-scope respected:** no Row-Policy-exempt CH user, no idle-tenant optimization, no aggregation SQL change, no Row Policy DDL / CH user / compose change (only the lint script + Go).
