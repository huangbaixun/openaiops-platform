---
date: 2026-05-31
topic: platform-mt-1-domain-switching-design
type: feature
status: proposed
features: [PLATFORM-MT-1]
adr: [0004]
---

# PLATFORM-MT-1 design — Domain-scoped multi-tenant switching

## Context

The platform's auth model today is **pure API-key bearer**: a request carries
`Authorization: Bearer <key>`, the middleware resolves the key to exactly one
tenant, and that single `tenant_id` is placed on the request context and drives
all isolation (`chquery.MustTenantScope` + ClickHouse Row Policy). There is **no
human-user concept** (no users/passwords/sessions/JWT — JWT is explicitly deferred
to v1.0+ per ADR-0001). The only cross-tenant mechanism is the `service:ai` scope +
`X-Tenant-Id` header (PLATFORM-ASK-1), which lets the AI service act on behalf of
**any** tenant. Tenants are flat-keyed by UUID — no domain/environment/org grouping.

PLATFORM-UI-1 built the topbar Domain/Project/Env scope-pill as a forward-compatible
read-only presentation layer with an explicit `FEATURE-B (PLATFORM-MT-1)` seam. This
feature (Feature B) wires that selector to **real** switching.

**Identity substrate decision (locked):** MT-1 uses **domain-scoped API keys**, not a
human-user/session system. A key may be bound to a *domain* (a named group of tenants)
and may switch among the tenants in that domain via the existing `X-Tenant-Id`
mechanism, generalized from `service:ai`. This reuses ASK-1's proven path, preserves
the day-1 "one active tenant per request" isolation invariant, and avoids building a
password/SSO/session layer that the pre-MVP platform does not yet need. A full human-
user identity model remains a future v1.0+ concern.

## Goals / non-goals

**Goals**
- Group tenants under named **domains**; tag each tenant with a deployment **environment**.
- Add a `domain` API-key scope that may switch among the tenants of *its own domain*
  (and only those) via `X-Tenant-Id`, fail-closed.
- Expose `GET /api/v1/tenants` returning the caller's visible tenant set so the frontend
  can populate the Project/Env selectors.
- Wire the PLATFORM-UI-1 scope-pill to real switching (no re-login; same key + header).
- Document the auth-model change in **ADR-0004**.
- Preserve the multi-tenant isolation invariant end-to-end (ADR-0001 §3.3).

**Non-goals (YAGNI)**
- Human users / passwords / SSO / sessions / JWT (excluded by the chosen light substrate).
- Environment as an access boundary (env is organizational metadata only).
- Domain/tenant-membership CRUD UI (domains + memberships are managed via
  migration/seed now; an admin UI is a separate future feature).
- Cross-domain switching (a `domain` key is confined to its own domain).

## Data model (PG, goose migration)

```sql
-- +goose Up
CREATE TABLE domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE tenants ADD COLUMN domain_id   UUID NULL REFERENCES domains(id) ON DELETE SET NULL;
ALTER TABLE tenants ADD COLUMN environment TEXT NULL;
CREATE INDEX idx_tenants_domain ON tenants(domain_id);
-- +goose Down
DROP INDEX IF EXISTS idx_tenants_domain;
ALTER TABLE tenants DROP COLUMN IF EXISTS environment;
ALTER TABLE tenants DROP COLUMN IF EXISTS domain_id;
DROP TABLE IF EXISTS domains;
```

- `domains`: a named group. `tenants.domain_id` NULL = ungrouped/legacy tenant.
- `tenants.environment`: free-form deployment-env tag (dev/sit/uat/prod/…), nullable.
- `api_keys`: **no new column.** A new `scope` value `domain` opts a key into switching.
  (Existing `read-write` / `service:ai` values unchanged; `scope` stays free-form TEXT.)

## Auth layer (generalize ASK-1)

After `ResolveBearer` resolves `(key, keyTenant)`, the middleware branches on `key.Scope`
(**fail-closed** — unknown/absent header ⇒ pin to key's own tenant):

| Scope | `X-Tenant-Id` behavior |
|---|---|
| `read-write` (and any other) | **Ignored.** Pinned to the key's own tenant. (unchanged) |
| `service:ai` | May target **any** tenant. (unchanged — AI write-back) |
| `domain` (new) | May target a tenant **iff** `target.domain_id == keyTenant.domain_id` and that domain_id is non-NULL. Else 403. |

Error contract for a `domain` key presenting `X-Tenant-Id`:
- malformed UUID → **400**
- tenant not found → **404**
- tenant exists but outside the key's domain (or key's tenant has NULL domain_id) → **403**
- header absent → default to the key's own tenant (200)

Implementation reuses the existing `TenantLookup` interface + `PGResolver.TenantByID`,
adding a domain-membership check. `TenantByID` (or a sibling) must also return the
tenant's `domain_id` so the middleware can compare. The resolved, validated target
tenant is placed on context via the existing `WithTenant`; **everything downstream
(`MustTenantScope`, Row Policy, handlers) is unchanged** — there is still exactly one
active `tenant_id` per request.

A new `auth` helper exposes the membership check as a small, testable unit:
`func tenantInDomain(target, keyTenant tenant.Tenant) bool` (both must share a non-NULL
`domain_id`). The `tenant.Tenant` struct gains a `DomainID uuid.UUID` (nullable → use
`uuid.Nil` for NULL) and `Environment string` field; `PGResolver` queries select them.

## New endpoints (gateway :8080, admin/identity plane)

Both live on gateway per ADR-0003 (identity/admin plane, PG-backed). **No Caddy change**:
`/api/v1/tenants*` is not in the query-prefix allowlist, so it falls through Caddy's
catch-all to gateway:8080.

### `GET /api/v1/tenants` — enumerate visible tenants

Returns the caller's **visible** tenant set for populating the topbar selectors:

- `domain`-scoped key → all tenants sharing the key's `domain_id`:
  `[{ "id", "name", "environment" }, ...]` (caller may switch among these).
- any other scope (incl. a `domain` key whose tenant has NULL domain_id) → just the
  caller's own single tenant (selector renders read-only single item — exactly the
  PLATFORM-UI-1 forward-compatible behavior).

The handler reads `auth.TenantID(ctx)` (the key's *own* tenant — enumeration is always
relative to the key's home domain, independent of any active switch) + the key scope,
looks up the domain peers, and returns them. PG queries are explicit-tenant/domain
scoped (no bare SQL).

### `POST /api/v1/tenants/switch` — record a switch (audit point)

Body `{ "tenant_id": "<uuid>" }`. Validates that the caller's key may switch to the
target (same `domain` membership rule as the middleware), writes **one** `audit_log`
row (actor key id, from-tenant, to-tenant, ts), and returns `200 {tenant_id, name,
environment}`. Out-of-domain / non-`domain`-key → **403**; unknown → 404; malformed → 400.

This is the **single audit point**: the frontend calls it once when the user picks a
tenant, then sets the active tenant locally. The per-request middleware (below) keeps
enforcing isolation on every request via `X-Tenant-Id` but writes **no** audit — auditing
per request would burn a row on every page refresh (the same anti-pattern CLAUDE.md
forbids for `metering_events`). Calling `/switch` is advisory for audit + pre-flight UX
validation; security does **not** depend on it (a caller skipping it still cannot reach
an out-of-domain tenant, because the middleware re-validates every request).

## Frontend wiring (activate the UI-1 scope-pill)

- **auth store** gains `activeTenantId: string | null` and `domainTenants: TenantOption[]`.
  On login/restore: after `/healthz`, fetch `GET /api/v1/tenants`; set `domainTenants`
  and default `activeTenantId` = the key's own tenant. Persist `activeTenantId` to
  `localStorage` (so a refresh keeps the selection) — but always re-validate against the
  fetched `domainTenants` (drop if no longer a member).
- **axios interceptor** (`api/client.ts`): when `activeTenantId` is set and differs from
  the key's own tenant, attach `X-Tenant-Id: <activeTenantId>`.
- **ScopePill** (replace the UI-1 read-only stub): Domain segment shows the domain name;
  **Project dropdown = `domainTenants`, selecting one sets `activeTenantId`** (→ all
  subsequent requests carry the new `X-Tenant-Id`); Env pill shows the active tenant's
  `environment`, and each Project option carries an env badge / is grouped by env.
- On switch, trigger a reload of the current view through the existing `useTimeWindow`
  `refreshTick` path so pages re-query under the new active tenant. No re-login.

## Data flow (a switch, end to end)

1. User picks tenant **B** in the ScopePill → frontend `POST /api/v1/tenants/switch {B}`.
2. Backend validates `B.domain_id == keyTenant.domain_id`, writes one audit_log row, 200.
   (Out-of-domain → 403; frontend keeps the old selection + toasts.)
3. On 200, store sets `activeTenantId = B`; interceptor adds `X-Tenant-Id: B` to every
   subsequent request (Bearer key unchanged).
4. Per-request middleware: scope `domain` → re-validate `B.domain_id == keyTenant.domain_id`
   → adopt B → `WithTenant(ctx, B, …)`. `MustTenantScope` + Row Policy scope all CH/PG
   reads to B. One active tenant, as always — and isolation holds even if `/switch` was
   skipped (the middleware is the real gate; `/switch` is audit + UX pre-flight).
5. A `domain` key sending `X-Tenant-Id: C` where C ∉ its domain → **403**, no data leak.

## Error handling

- Switch to a tenant that has since left the domain → backend 403; frontend drops it from
  `domainTenants`, falls back to the key's own tenant, surfaces a toast.
- `GET /api/v1/tenants` failure on login → selector degrades to read-only single (own
  tenant); login still succeeds.
- A `read-write` key can never switch (header ignored) — the selector shows a single
  read-only item even if the user crafts a header manually.

## Security

- **ADR-0004** records the auth-model change (X-Tenant-Id generalized from `service:ai`
  to `domain` keys, scoped by domain membership; new domain grouping). Load-bearing per
  ADR-0001 §3.3.
- Fail-closed at every branch; domain membership requires a **non-NULL shared** domain_id
  (two NULL-domain tenants are NOT "in the same domain").
- **Audit**: written by the `POST /api/v1/tenants/switch` handler — **one row per switch
  action**, NOT per request (per-request audit would burn a row on every page refresh, the
  metering anti-pattern). Row = actor key id, from-tenant, to-tenant, ts. The `audit_log`
  table is created by this feature's migration (absent today), scoped by tenant_id.
- `service:ai` semantics are untouched (still any-tenant) — MT-1 only adds the `domain`
  branch.

## Testing strategy

- **Unit (auth middleware):** `domain` key + in-domain X-Tenant-Id → adopts target;
  `domain` key + out-of-domain → 403; `domain` key + own-tenant-NULL-domain → 403;
  `domain` key no header → own tenant; `read-write` key + header → ignored;
  malformed/unknown → 400/404. Plus `tenantInDomain` table tests.
- **Integration (dockertest PG):** `PGResolver` returns `domain_id`/`environment`;
  `GET /api/v1/tenants` returns domain peers for a domain key, single for a normal key.
- **Reverse E2E (CI gate, preserves ADR-0001 invariant):** domain key switch A→sees only
  A, switch B→sees only B; switch to out-of-domain C → 403; existing "A writes / B reads
  → 0 rows" still green.
- **Frontend (vitest + Playwright):** ScopePill lists domain tenants, switching changes
  the active tenant and re-queries; read-write key shows single read-only item.
- **Migration:** goose up/down round-trips; existing tenants get NULL domain_id (no break).

## Acceptance criteria

1. `domains` table + `tenants.domain_id` + `tenants.environment` migrated (goose up/down);
   existing tenants survive with NULL domain_id.
2. A `domain`-scoped key may switch among tenants of its own domain via `X-Tenant-Id`;
   out-of-domain target → 403; absent header → own tenant; `read-write`/`service:ai`
   behavior unchanged.
3. `GET /api/v1/tenants` returns the caller's domain peers (domain key) or its single
   tenant (other), each with `{id, name, environment}`; `POST /api/v1/tenants/switch`
   validates membership, writes one audit_log row, returns 200 (in-domain) / 403 (out).
4. The PLATFORM-UI-1 ScopePill is wired: Project dropdown switches the active tenant
   (no re-login), Env shows/groups by environment; pages re-query under the new tenant.
5. Isolation invariant preserved: reverse E2E (in-domain switch sees only target;
   out-of-domain blocked; A-writes/B-reads→0) green in CI.
6. ADR-0004 written documenting the auth-model change; each switch writes an audit_log row.
7. All existing tests stay green; new unit/integration/e2e for the above pass.

## Out of scope

- Human users / passwords / SSO / sessions / JWT.
- Domain & membership management UI (seed/migration-managed for now).
- Cross-domain switching; environment-based access control.

## Dependencies

- Builds on PLATFORM-ASK-1 (`X-Tenant-Id` + `TenantLookup` + `PGResolver.TenantByID`).
- Activates the PLATFORM-UI-1 ScopePill seam.

## Related files

- `backend/migrations/<new>_create_domains_tenant_columns.sql` (new)
- `backend/internal/auth/middleware.go` (add `domain` branch + membership check)
- `backend/internal/auth/resolver_pg.go`, `backend/internal/tenant/*` (DomainID/Environment)
- `backend/cmd/gateway/*` + a tenants handler (new `GET /api/v1/tenants`)
- `backend/migrations/<new>_create_audit_log.sql` (if audit_log absent)
- `frontend/src/stores/auth.ts`, `frontend/src/api/client.ts`, `frontend/src/api/tenants.ts` (new),
  `frontend/src/components/ScopePill.vue`
- `deploy/seed.sql` (a domain + a `domain`-scoped key)
- `docs/decisions/0004-domain-scoped-tenant-switching.md` (new ADR)
