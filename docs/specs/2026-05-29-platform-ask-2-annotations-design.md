---
date: 2026-05-29
topic: platform-ask-2-annotations-design
type: feature
status: proposed
features: [PLATFORM-ASK-2]
---

# PLATFORM-ASK-2 design — `/api/v1/annotations` write-back endpoint

## Context

`openaiops-ai` (sibling repo) needs to write RCA findings back onto the platform's
trace/service/topology nodes and have them surface in the platform UI. This is the
SLICE-1 write-back contract from the openaiops-ai design (§5.1, §6, §11).

Depends on **PLATFORM-ASK-1** (shipped, `done_with_concerns`, commit `94e3299`): the auth
middleware already resolves the *effective tenant* into request context — a `service:ai`
key plus `X-Tenant-Id: <T>` yields tenant `T` in `ctx`, every other scope is pinned to the
key's own tenant. ASK-2 reuses that exact mechanism as its cross-tenant-write guard: it
never trusts a tenant id from the request body, only `auth.TenantID(ctx)`.

Brainstormed 2026-05-29 (this session). **Four design choices locked** via per-decision
multi-choice picks:

1. **Binary placement** = both POST + GET on `cmd/query` (:8081). One Caddy `handle`,
   co-located with the trace/service/topology reads the badges attach to. `query` already
   holds a PG handle (used for auth), so no new dependency. This is a documented deviation
   from ADR-0003 (query gains its first write endpoint) — see §7.
2. **Idempotency** = unbounded `UNIQUE(tenant_id, idempotency_key)` + return-existing on
   conflict. Dedup never produces duplicates; the "24h" in the AC is treated as a retention
   *floor* (we keep keys indefinitely, which strictly satisfies "dedupe within 24h").
   Key-reuse-after-24h is explicitly out of scope.
3. **Frontend scope** = full coverage: badge + click-to-detail modal on all three node
   types (trace detail, service detail, topology graph node), following the OpenAPM
   hi-fi design at https://huangbaixun.github.io/OpenAPM/.
4. **Target taxonomy** = `target_type ∈ {trace, service}` only. Topology/overview graph
   nodes reuse `service` annotations keyed by service name — no separate `edge` target.

## Data model (PostgreSQL, goose)

New migration `backend/migrations/20260529NNNNNN_create_annotations.sql`:

```sql
-- +goose Up
CREATE TABLE annotations (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  target_type     TEXT         NOT NULL CHECK (target_type IN ('trace','service')),
  target_id       TEXT         NOT NULL,   -- trace_id (hex) or service name
  kind            TEXT         NOT NULL,   -- 'ai_rca'; open set, not an enum
  payload         JSONB        NOT NULL,
  ts              TIMESTAMPTZ  NOT NULL,   -- event time, client-supplied
  idempotency_key TEXT         NULL,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_annotations_lookup ON annotations(tenant_id, target_type, target_id);
CREATE UNIQUE INDEX uq_annotations_idem ON annotations(tenant_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS annotations;
```

This is a PG table (relational metadata, low volume), **not** a CH table — so it uses goose,
not the CH runner, and does not involve `chquery` / Row Policy. Multi-tenancy is enforced in
the handler: every read/write is scoped to `auth.TenantID(ctx)` via parameterized SQL (no
bare string interpolation).

## Write endpoint — `POST /v1/annotations`

(Public path `/api/v1/annotations`; Caddy strips `/api`.)

Request body:
```json
{ "target_type": "service", "target_id": "checkout",
  "kind": "ai_rca", "payload": { "...": "..." },
  "ts": "2026-05-29T12:00:00Z", "tenant_id": "optional-uuid" }
```

Behavior:
- Effective tenant = `auth.TenantID(ctx)` (resolved by ASK-1 middleware).
- If body includes `tenant_id` and it ≠ ctx tenant → **403** (AC#3 cross-tenant write block).
  Insert always uses the ctx tenant; the body field is accepted only as a redundant assertion.
- Validate: `target_type ∈ {trace, service}`, `kind` non-empty, `payload` present, `ts`
  RFC3339-parseable. Otherwise **400**.
- `Idempotency-Key` request header (optional) → stored as `idempotency_key`.
- `INSERT … ON CONFLICT (tenant_id, idempotency_key) DO NOTHING RETURNING id`. If no row is
  returned (conflict on a non-null key), `SELECT id FROM annotations WHERE tenant_id=$1 AND
  idempotency_key=$2`.
- Response: **201** `{"annotation_id": "<uuid>"}` on create; **200** `{"annotation_id":
  "<uuid>"}` on idempotent hit (same id).

## Read endpoint — `GET /v1/annotations`

Query params:
- `target_type` — required, `∈ {trace, service}`.
- `target_id` — optional. If present, returns annotations for that one target (trace detail
  page). If omitted, returns all annotations of that type for the tenant (topology / services
  pages fetch all `service` annotations in one call).
- `limit` — default 100, capped 500.

Always filtered by ctx tenant. Returns `[{id, target_type, target_id, kind, payload, ts,
created_at}]` ordered by `ts DESC`.

## Caddy routing

Add one `handle` block **before** the `/api/*` catch-all (first-match-wins), mirroring the
SLICE-3 one-liners — zero `frontend/nginx.conf` changes (drift D4 stays closed):

```
handle /api/v1/annotations* {
  uri strip_prefix /api
  reverse_proxy query:8081
}
```

## Frontend (follows OpenAPM hi-fi design)

- `frontend/src/api/annotations.ts` — **must** import the shared `api/client.ts` axios
  instance (SLICE-3 T15 regression: raw `fetch` skips the Bearer interceptor → 401). Tests
  assert `client.get` / `client.post` were called, not a raw URL string.
- `useAnnotations(targetType, targetId?)` composable — fetch + cache for a page.
- `<AnnotationBadge>` — small badge (count + kind icon); click opens an `NModal` detail layer
  rendering kind, pretty-printed `payload` JSON, and `ts`.
- Wired into three places: TraceDetail header, ServiceDetail page, and `<ServiceGraph>` node
  decoration (reused by both the Topology page and the Overview map).

## ADR-0003 deviation (documented, no new ADR)

ADR-0003 split the binaries as gateway = write/admin plane, query = CH read path. ASK-2 adds
the first **write** endpoint to `cmd/query`. Rationale for placing it on query anyway:
annotations are low-volume PG metadata writes (not the high-throughput ingest writes ADR-0003
was protecting gateway CPU from), and they are read in the same request shape and same process
as the trace/service/topology data the badges attach to. Splitting POST→gateway / GET→query
would require a Caddy method matcher and have two binaries touch one table for no real benefit.
Per user decision (2026-05-29) this deviation is recorded here in the spec rather than opening
a formal ADR. A one-line amendment pointer will be added to ADR-0003.

## Testing strategy

Backend:
- `annotations_handler_test.go` — unit, fake repo: 201 create, 200 idempotent hit, 403
  cross-tenant write, 400 validation (bad target_type / missing kind / unparseable ts), GET
  by target_id, GET all-of-type.
- `annotations_repo_test.go` — dockertest integration (`-tags=integration`): Insert,
  idempotency conflict returns same id, tenant isolation (tenant A write not visible to
  tenant B read). ⚠️ If the dev machine's docker daemon is down (as on 2026-05-29), this is
  compile-verified only and the feature lands `done_with_concerns` with a drift note — same
  posture as ASK-1's D7.

Frontend:
- vitest — `annotations.ts` routes through the shared client; `useAnnotations` composable;
  `<AnnotationBadge>` renders count + opens modal.
- Playwright e2e — badge appears on trace detail, service detail, and a topology node; modal
  opens with payload.

## Acceptance criteria mapping (features.json PLATFORM-ASK-2)

1. `POST … returns 201 + annotation_id` → write endpoint §Write.
2. `Annotation visible as badge on trace/service/topology node` → frontend §Frontend (full
   three-place coverage).
3. `Cross-tenant write blocked (tenant_id must match bearer scope or X-Tenant-Id)` → §Write
   403 rule, reusing ASK-1's ctx tenant.
4. `Idempotency-Key header dedupes within 24h` → §Idempotency, unbounded unique + return
   existing (satisfies "within 24h" as a retention floor).

## Out of scope

- UI editing of annotations; annotation comments/threads (per features.json).
- Key-reuse after 24h (idempotency keys retained indefinitely for MVP).
- `edge` target type (topology nodes reuse `service` annotations).
- A periodic prune job for old idempotency keys (follow-up if the table grows).
