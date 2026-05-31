---
date: 2026-05-31
topic: platform-topo-1-pg-tenant-discovery-design
type: feature
status: proposed
features: [PLATFORM-TOPO-1]
adr: [0005]
---

# PLATFORM-TOPO-1 design — topo-engine PG-driven tenant discovery (fix D6)

## Context

topo-engine aggregates `traces_v1` into `service_stats_v1` (service RED metrics) and
`topology_edges_v1` (service-graph edges) on a per-minute tick. Its **aggregation writes
already work**: `edges.go`/`services.go` run per-tenant via the tenant-scoped
`chquery.Conn.Exec` with the real `tenant_id` in `custom_tenant_id`, which passes the
`tenant_isolation_*` Row Policy.

The broken part is **tenant discovery**. Two queries route through `chquery.AdminConn`,
which injects a sentinel `custom_tenant_id = ""` (`backend/internal/chquery/admin.go:22`):

- `activeTenants()` → `SELECT DISTINCT tenant_id FROM traces_v1 WHERE ts >= ?`
- `lastCompletedBucket()` → `AdminMaxBucket`: `SELECT max(ts_bucket) FROM topology_edges_v1 FINAL WHERE tenant_id = ?`

Each `tenant_isolation_*` Row Policy is `USING tenant_id = getSetting('custom_tenant_id') TO openaiops`.
With the sentinel `''`, the policy evaluates `tenant_id = ''` → **zero rows**. So:
`activeTenants()` returns no tenants → topo-engine aggregates nothing → `service_stats_v1`
and `topology_edges_v1` stay empty → `GET /api/v1/services` returns `[]` → the
PLATFORM-UI-1 `shell.spec` ⌘K e2e (and any topology/services UI) shows nothing.
`lastCompletedBucket()` likewise returns 0, so Catchup silently re-scans from scratch.

This is tracked as **known_drift D6**. The codebase's operator note
(`cmd/topo-engine/main.go:6-12`) anticipated fixing this by granting topo-engine a
Row-Policy-exempt CH user. We instead choose a cleaner path (decision locked with the user
2026-05-31): **drive tenant discovery from PostgreSQL.**

topo-engine **already opens a PG connection** (`cmd/topo-engine/main.go:55`, passed as
`Deps.PG`) that is currently unused ("reserved for future"). The `tenants` table is the
authoritative tenant registry. Reading the tenant list from PG and then running the
existing per-tenant scoped CH aggregation keeps topo-engine fully inside the isolation
model — it never does a cross-tenant CH read, and there is no Row-Policy-exempt user to
secure.

## Goals / non-goals

**Goals**
- Discover the tenant set from PG (`tenants` table) via the already-open `Deps.PG`.
- Make `lastCompletedBucket()` a normal per-tenant scoped CH query (it already carries
  `tenant_id = ?`).
- Remove the now-unused `chquery.AdminConn` + sentinel mechanism + its lint exception.
- Fix D6 end-to-end: `service_stats_v1`/`topology_edges_v1` populate; the ⌘K e2e goes green.
- Touch **no** Row Policy DDL, **no** CH user/grants, **no** deploy config.

**Non-goals (YAGNI)**
- Optimizing idle-tenant aggregation (currently ~5 tenants; every tick re-aggregates all,
  idle ones produce 0 rows at negligible cost). A "tenants with traces in window" filter is
  a future optimization, not needed now.
- Changing the aggregation SQL (`edges.go`/`services.go`) — it is correct and unchanged.
- Any Row Policy / CH user / `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT` change.

## Approach

### Tenant discovery → PG

Replace the `AdminConn`-based `activeTenants()` with a PG read of the `tenants` table.
A small repo method on the engine (using `e.deps.PG`):

```sql
SELECT id FROM tenants
```

Returns all registered tenant UUIDs. The previous `since time.Time` argument (which
restricted to tenants with recent trace activity) is dropped from the semantics — we
enumerate all tenants; idle ones aggregate to zero rows. `activeTenants(ctx)` keeps a
similar signature for its callers (`RunBucket`, `Catchup`) but sources from PG. The
per-tenant aggregation that follows is unchanged and already passes the Row Policy.

### lastCompletedBucket → tenant-scoped Conn

`AdminMaxBucket` (`max(ts_bucket) FROM topology_edges_v1 FINAL WHERE tenant_id = ?`) is
already per-tenant. Move it to a normal `chquery.Conn.Query` executed in a context carrying
that tenant's id (so `MustTenantScope` injects `tenant_id = ?` and `custom_tenant_id` is the
real tenant). This passes the Row Policy and returns the true max bucket, so Catchup seeds
correctly instead of re-scanning from the catchup-window start each boot.

### Remove AdminConn

With both admin queries gone, delete `backend/internal/chquery/admin.go` (the `AdminConn`
type, `adminSentinelTenantID`, `adminCtx`, `AdminQueryKind` whitelist), drop `Admin` from
`topoengine.Deps`, stop constructing it in `cmd/topo-engine/main.go`, and remove the
AdminConn-related exception in `deploy/lint-no-bare-ch.sh` (the lint that restricts bare CH
imports — confirm the topoengine package still satisfies it via `chquery.Conn`).

## Data flow (one tick, after the fix)

1. `RunBucket(bucket)` calls `activeTenants(ctx)` → PG `SELECT id FROM tenants` → `[t1, t2, …]`.
2. For each tenant (bounded by `TOPO_TENANT_CONCURRENCY`), run the existing Pass A (edges)
   and Pass B (services) via `e.deps.CH.Exec(ctx-with-tenant, sql, tid, bucketStart, bucketEnd)`.
   Each carries the real `custom_tenant_id` → Row Policy passes → rows written.
3. Catchup seeds each tenant's start bucket from `lastCompletedBucket(ctx, tid)` (now a
   tenant-scoped read of `topology_edges_v1`).

One active tenant per CH query, exactly as the isolation model requires. No cross-tenant
read anywhere.

## Error handling

- PG `SELECT id FROM tenants` failure → the tick logs and is skipped (same resilience as the
  prior AdminConn error path); the next tick retries. A transient PG blip does not crash the
  engine.
- A per-tenant aggregation error isolates to that tenant (existing behavior) — other tenants
  still process.

## Security

- **Strengthens** the isolation posture: removes the only Row-Policy-bypass mechanism in the
  codebase (`AdminConn` sentinel). No privileged CH user is introduced. The `tenant_isolation_*`
  policies remain `TO openaiops` with no exemptions.
- topo-engine reads tenant IDs (not tenant data) from PG — the `tenants` table is metadata it
  is already entitled to (it shares the `openaiops` PG database).

## Testing strategy

- **Integration (dockertest PG + CH):** seed N tenants in PG + traces in CH for some of them;
  assert `activeTenants` returns the PG tenant set; assert `RunBucket` writes
  `service_stats_v1`/`topology_edges_v1` rows for tenants that have traces and none for idle
  ones; assert `lastCompletedBucket` returns the real max bucket for a tenant. (Note: the test
  CH user has `ACCESS_MANAGEMENT=1` which masks the Row Policy, so these tests validate the PG
  path + aggregation, not the policy bypass per se.)
- **D6 end-to-end (the real acceptance):** on a fresh dev stack, rebuild the topo-engine image
  (the `make up` doesn't `--build` lesson), `make seed`, seed traces + topology at
  `localhost:14317`, wait one tick (~120s), confirm `GET /api/v1/services` returns services,
  and **`shell.spec` ⌘K e2e passes** — closing D6.
- **Regression:** full backend unit + integration green; full Playwright suite green
  (34/34 including the previously-red ⌘K).

## Acceptance criteria

1. `activeTenants` sources the tenant list from PG (`tenants` table) via `Deps.PG`; the
   AdminConn-based discovery is gone.
2. `lastCompletedBucket` is a tenant-scoped CH query (passes the Row Policy, returns the real
   max bucket).
3. `chquery.AdminConn` + sentinel + the lint exception are removed; topo-engine builds and the
   no-bare-CH lint still passes.
4. On a fresh stack (policy enforced for `openaiops`), after seed + one tick,
   `service_stats_v1`/`topology_edges_v1` populate and `GET /api/v1/services` returns data.
5. The `shell.spec` ⌘K e2e passes; full Playwright suite is green; D6 is closed in known_drift.
6. No Row Policy DDL, CH user, or deploy-config change. ADR-0005 documents the decision.

## Out of scope

- Row-Policy-exempt CH user (Option A — explicitly rejected).
- Idle-tenant aggregation optimization.
- Aggregation SQL changes.

## Dependencies

None. Self-contained topo-engine + chquery change.

## Related files

- `backend/internal/topoengine/tenants.go` (activeTenants → PG), `types.go` (Deps.Admin removed)
- `backend/internal/topoengine/edges.go` / `services.go` (unchanged aggregation; lastCompletedBucket caller)
- `backend/internal/chquery/admin.go` (deleted)
- `backend/cmd/topo-engine/main.go` (stop building AdminConn; remove operator note)
- `deploy/lint-no-bare-ch.sh` (remove AdminConn exception)
- `docs/decisions/0005-topo-engine-pg-tenant-discovery.md` (new ADR)
