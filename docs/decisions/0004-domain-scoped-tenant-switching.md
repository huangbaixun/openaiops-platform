# ADR-0004: Domain-scoped multi-tenant switching

- **Status**: Accepted
- **Date**: 2026-05-31
- **Deciders**: @huangbaixun
- **Tracks**: PLATFORM-MT-1
- **Full design**: `docs/specs/2026-05-31-platform-mt-1-domain-switching-design.md`
- **Builds on**: ADR-0001 §3.3 (row-level multi-tenant isolation), PLATFORM-ASK-1 (`service:ai` + `X-Tenant-Id`)

## Context

The platform authenticates with bearer API keys; each key resolves to exactly one
tenant, and that single `tenant_id` on the request context drives all isolation
(`chquery.MustTenantScope` + ClickHouse Row Policy — ADR-0001 §3.3, load-bearing).
There is no human-user/session model (JWT deferred to v1.0+). The only cross-tenant
path is the `service:ai` scope + `X-Tenant-Id` header (PLATFORM-ASK-1), which grants
access to **any** tenant. Tenants are flat — no grouping.

PLATFORM-MT-1 must let an operator view and switch among **multiple** tenants (the
OpenAPM Domain/Project/Env topbar). We must decide the identity substrate and the
switching mechanism **without weakening the day-1 isolation invariant**.

Options considered:

- **A — Human users + sessions:** new `users` table, password/SSO login, sessions/JWT,
  `user↔tenant` membership ACL, switch re-issues a scoped session. The "correct" SaaS
  model, but a large departure from the API-key substrate and premature for pre-MVP
  (JWT already deferred to v1.0+).
- **B — Domain-scoped API key (chosen):** introduce a *domain* (named group of tenants);
  a key with the new `domain` scope may switch among the tenants of *its own domain* via
  the existing `X-Tenant-Id` header, validated by domain membership. No user/session layer.
- **C — Generalize `service:ai` to all keys:** let any key switch via `X-Tenant-Id`.
  Rejected — silently widens every existing `read-write` key to cross-tenant access; unsafe.

## Decision

Adopt **Option B — domain-scoped API keys.**

1. **Domain grouping.** New PG `domains` table; `tenants` gains `domain_id` (nullable FK)
   and `environment` (free-form deployment-env tag). NULL `domain_id` = ungrouped/legacy.

2. **`domain` scope.** A new `api_keys.scope` value `domain` (alongside `read-write`,
   `service:ai`). The middleware honors `X-Tenant-Id` for a `domain` key **iff** the
   target tenant shares the key's tenant's **non-NULL** `domain_id`; otherwise 403.
   `read-write` (and any other) keys continue to ignore the header; `service:ai` keeps
   its any-tenant power. Branch is **fail-closed** — absent/invalid header pins to the
   key's own tenant.

3. **Invariant preserved.** The resolved, domain-validated target tenant is placed on
   context via the existing `WithTenant`. Downstream — `MustTenantScope`, Row Policy,
   handlers — is unchanged. There is still **exactly one active `tenant_id` per request**.
   The switch happens at auth time, not in the data layer.

4. **Enumeration endpoint.** `GET /api/v1/tenants` (gateway, identity/admin plane per
   ADR-0003) returns the caller's domain peers (domain key) or its single tenant (other),
   so the frontend can populate the selector. Switching uses the same key + `X-Tenant-Id`;
   **no re-login**.

5. **Audit.** Each successful switch (a `domain` key adopting a target ≠ its own tenant)
   writes an `audit_log` row.

## Consequences

**Positive**
- Reuses the proven ASK-1 mechanism; minimal auth-layer surface; no new credential/session
  system to secure.
- Isolation invariant (ADR-0001 §3.3) is structurally preserved — one active tenant per
  request, same `MustTenantScope` + Row Policy enforcement; the cross-tenant reverse E2E
  remains the CI gate and is extended (in-domain switch sees only target; out-of-domain
  blocked).
- Maps cleanly onto the user's model: Domain = tenant group, Project = tenant, Environment
  = per-tenant deploy tag.

**Negative / risks**
- Widens `X-Tenant-Id` honoring beyond `service:ai`. Mitigated by strict, fail-closed
  domain-membership validation (non-NULL shared `domain_id` required) and dedicated
  out-of-domain-→403 tests.
- `scope` remains free-form TEXT; a typo'd scope silently behaves as `read-write`
  (pinned). Acceptable for now; a CHECK constraint or enum is a future hardening.
- Does **not** deliver human users / per-user RBAC. That remains a future v1.0+ feature
  layered above this (a user would map to a domain-scoped key today).

**Superseded if** the platform adopts the human-user/session model (v1.0+): domain
membership would move from "key's tenant's domain" to "user's tenant memberships," and
this ADR's key-centric mechanism would be revisited.

## Compliance / verification

- Reverse E2E (CI gate): in-domain switch isolation, out-of-domain 403, and the existing
  A-writes/B-reads→0 must all pass.
- `make lint-ch` / no-bare-SQL rules unchanged — the data layer is untouched.
