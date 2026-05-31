# ADR-0005: topo-engine PG-driven tenant discovery (retire AdminConn)

- **Status**: Proposed
- **Date**: 2026-05-31
- **Deciders**: @huangbaixun
- **Tracks**: PLATFORM-TOPO-1 (fixes known_drift D6)
- **Full design**: `docs/specs/2026-05-31-platform-topo-1-pg-tenant-discovery-design.md`
- **Supersedes**: the `chquery.AdminConn` sentinel mechanism (`backend/internal/chquery/admin.go`) and the operator note in `backend/cmd/topo-engine/main.go`.
- **Relates to**: ADR-0001 §3.3 (row-level multi-tenant isolation, load-bearing).

## Context

topo-engine discovers which tenants to aggregate by running `SELECT DISTINCT tenant_id
FROM traces_v1` through `chquery.AdminConn`, which injects a sentinel `custom_tenant_id = ""`.
The `tenant_isolation_*` Row Policies (`USING tenant_id = getSetting('custom_tenant_id') TO
openaiops`) then evaluate `tenant_id = ''` → zero rows. topo-engine therefore discovers no
tenants and never populates `service_stats_v1` / `topology_edges_v1` (known_drift **D6**).
The same affects `lastCompletedBucket` (`AdminMaxBucket`).

The original design (operator note in `cmd/topo-engine/main.go`) anticipated fixing this in
production by granting topo-engine a CH user **exempt** from the Row Policies.

Options:

- **A — Row-Policy-exempt CH user:** create `topo_admin`, `ALTER ROW POLICY … TO openaiops,
  topo_admin`, switch topo-engine's CH DSN. The path the operator note assumed.
- **B — PG-driven tenant discovery (chosen):** read the tenant list from the authoritative
  PG `tenants` table (via topo-engine's already-open, currently-unused PG connection), then
  run the existing per-tenant **scoped** CH aggregation. Retire `AdminConn` entirely.
- **C — Robust-test-only:** make the ⌘K e2e not depend on `service_stats`. Rejected — leaves
  D6 unfixed (topo-engine still produces no data in production).

## Decision

Adopt **Option B — PG-driven tenant discovery.**

1. `activeTenants()` reads `SELECT id FROM tenants` from PG (`Deps.PG`) instead of the
   AdminConn `SELECT DISTINCT tenant_id FROM traces_v1`.
2. `lastCompletedBucket()` becomes a normal **tenant-scoped** `chquery.Conn` query (it already
   carries `tenant_id = ?`), passing the Row Policy with the real `custom_tenant_id`.
3. `chquery.AdminConn`, `adminSentinelTenantID`, `adminCtx`, the `AdminQueryKind` whitelist, and
   the corresponding `deploy/lint-no-bare-ch.sh` exception are **removed**.
4. The per-tenant aggregation (`edges.go` / `services.go`) is unchanged — it already runs
   scoped and passes the Row Policy.

## Consequences

**Positive**
- Fixes D6 with **zero** change to Row Policy DDL, CH users, or deploy config — and no
  privileged credential to manage or leak.
- **Strengthens** isolation: removes the only Row-Policy-bypass mechanism in the codebase.
  topo-engine becomes a pure in-model consumer — N per-tenant scoped reads, each with the
  correct `custom_tenant_id`. The day-1 isolation invariant (ADR-0001 §3.3) is reinforced,
  not weakened.
- Uses the already-open PG connection; no new dependency.
- Discovery from the authoritative `tenants` registry is more correct than "tenants seen in
  traces lately" (a tenant with retained-but-aged data is still enumerated).

**Negative / trade-offs**
- Every tick re-aggregates **all** tenants, including idle ones (which produce zero rows). At
  the current scale (~5 tenants) this is negligible. At large idle-tenant counts it would waste
  cycles; a "tenants with traces in the window" pre-filter is a future optimization (out of
  scope, noted in the spec).
- topo-engine now hard-depends on PG for discovery (previously PG was opened but unused). This
  is acceptable — topo-engine already requires PG to be healthy (compose `depends_on`).

**Superseded mechanism**
- `chquery.AdminConn` and its sentinel were built specifically to let an admin path bypass the
  Row Policy. With PG-driven discovery there is no admin CH path, so the mechanism (and its
  prod "grant exemption" caveat) is retired.

## Compliance / verification

- The cross-tenant reverse E2E (A-writes / B-reads → 0) remains the CI gate and is unaffected
  (no policy/user change).
- D6 acceptance: on a fresh stack, after seed + one tick, `GET /api/v1/services` returns data
  and the `shell.spec` ⌘K e2e passes.
- `make lint-ch` / no-bare-CH still passes after `AdminConn` removal.
