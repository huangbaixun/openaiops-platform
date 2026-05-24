---
date: 2026-05-24
topic: pre-2-query-api-deployment
type: adr-proposal
status: proposed
adr: [0003]
---

# PRE-2 design — Query API deployment shape

## Context

SLICE-1 brings the first ClickHouse read path (`/api/v1/traces`). PRE-2 (tracked in `docs/claude-progress.json` open_tasks at session-start) gates how query handlers are deployed: extend the existing `cmd/gateway` binary, or split into a separate `cmd/query` service.

User-confirmed assumptions:

1. **Workload shape:** Heavy. Server-side aggregation / long scans known to be coming (topology graph traversal, metric rollups, log scans).
2. **Timing:** Heavy workload arrives in SLICE-1 / 2 (within ~1 month), not deferred to SLICE-4+.

Together these refute progress.json's original Option A recommendation: under Heavy-soon, the cost of a future split (1-2d) is paid back the first time a query saturates gateway CPU and impacts the ingest auth path. We pay the split cost now, before any production traffic is at stake.

## Decision

**Option B — separate `backend/cmd/query/` service from day 1.** Gateway keeps the write/admin/auth ingestion path; query owns all CH read endpoints. Shared concerns live in `internal/` packages imported by both binaries.

The full alternatives explored: A (extend gateway), B (split now — chosen), C (factory under `pkg/queryapi` mounted by gateway, deferred split). C was attractive when split timing was uncertain, but with Heavy-soon confirmed it adds a transitional abstraction we throw away in weeks.

## Architecture

### Code layout

```
backend/
├── cmd/
│   ├── gateway/   (existing, SLICE-0)
│   └── query/     (new)
├── internal/
│   ├── auth/      (shared — bearer middleware + ctx injection)
│   ├── apikey/    (shared — PG resolver)
│   ├── tenant/    (shared — domain types)
│   ├── config/    (shared — gains LoadQuery())
│   ├── httpsrv/   (shared — gains httpsrv.Run() shutdown template)
│   ├── chquery/   (new — MustTenantScope helper + CH client pool; PRE-3 implements)
│   └── query/     (new — query-only handlers: traces, logs, services, topology)
└── ch-migrations/ (PRE-1, ready for SLICE-1 DDL)
```

### Route partition (decided by Caddy, not by binary)

| Path prefix | Routes to | Reason |
|---|---|---|
| `/api/v1/traces*`, `/api/v1/logs*`, `/api/v1/services*`, `/api/v1/topology*` | `query:8081` | All CH read paths |
| `/api/v1/admin*`, `/api/v1/metering*`, `/healthz`, `/livez` | `gateway:8080` | PG + admin |
| frontend assets | `frontend:80` (via nginx) | unchanged |

Both binaries register their handlers under the **same external URL space** — Caddy uses `handle` (not `handle_path`), so the prefix is preserved end-to-end. This keeps logs and traces correlatable across the proxy.

### Auth

Both binaries run `auth.Middleware(apikey.NewPGResolver(pg))`. Each maintains its own PG connection pool. Trade-off explicit: double PG connections (1 pool per binary) for cleaner failure isolation — if gateway's PG pool exhausts, query keeps serving; vice versa. Acceptable at ≤10 tenants / pre-prod scale.

### Compose service (`deploy/docker-compose.yml`)

```yaml
query:
  build: ../backend
  command: ["/usr/local/bin/query"]
  environment:
    DATABASE_URL: "postgres://openaiops:openaiops@postgres:5432/openaiops?sslmode=disable"
    CLICKHOUSE_DSN: "tcp://openaiops:openaiops@clickhouse:9000/openaiops"
    QUERY_LISTEN_ADDR: ":8081"
  ports: ["127.0.0.1:${QUERY_HOST_PORT:-8081}:8081"]
  depends_on:
    postgres:   { condition: service_healthy }
    clickhouse: { condition: service_healthy }
    migrate:    { condition: service_completed_successfully }
    ch-migrate: { condition: service_completed_successfully }
  healthcheck:
    test: ["CMD", "wget", "-qO-", "http://localhost:8081/livez"]
    interval: 5s
    timeout: 3s
    retries: 10
```

`gateway` service `command` becomes explicit `/usr/local/bin/gateway` (now that two binaries exist).

### Dockerfile (multi-binary)

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /out/gateway ./cmd/gateway && \
    go build -o /out/query   ./cmd/query

FROM alpine:3.19
COPY --from=builder /out/gateway /out/query /usr/local/bin/
```

Single image, two binaries. Compose picks via `command:`. Avoids duplicate build layers across services.

### Caddyfile

```caddy
:80 {
    handle /api/v1/traces*    { reverse_proxy query:8081 }
    handle /api/v1/logs*      { reverse_proxy query:8081 }
    handle /api/v1/services*  { reverse_proxy query:8081 }
    handle /api/v1/topology*  { reverse_proxy query:8081 }

    handle /api/*             { reverse_proxy gateway:8080 }
    handle                    { reverse_proxy frontend:80 }
}
```

Specificity-ordered: query routes first, then `/api/*` catch-all to gateway, then default to frontend. `handle` (not `handle_path`) preserves prefix.

### Health probes

| Binary | `/livez` | `/healthz` |
|---|---|---|
| gateway | 200 always (process up) | checks PG round-trip |
| query   | 200 always (process up) | checks PG (for auth) + CH (for reads) |

## Cross-cutting

Division of responsibility:

| Layer | Owned by | What |
|---|---|---|
| Process lifecycle | `httpsrv.Run(name, addr, handler)` | listen, signal trap, graceful shutdown, process-level slog setup (JSON handler, level from env) |
| Generic HTTP middleware | each `main` before calling `Run` | `chi/middleware.RequestID`, `chi/middleware.Recoverer` |
| Auth middleware | each `main` before calling `Run` | `auth.Middleware(resolver)` — needs the per-binary PG resolver injected |
| Route mounting | each `main` | gateway-only or query-only handlers |

So `httpsrv.Run` is the **process template** (lifecycle + logging), not a middleware chain. Each main composes its own chi router, attaches middleware, mounts routes, then hands the finished `http.Handler` to `Run`. Both mains end up with identical generic-middleware blocks — a future small helper (e.g. `httpsrv.NewMux()`) could de-dupe those 4-5 lines if it ever becomes annoying.

Per the decision: **metering is only on the write path (gateway)**. Query requests do not write `metering_events` — refreshing a page would otherwise burn quota and create a frustrating product feel. Read-side billing reconsideration deferred to post-MVP (v1.x).

## MustTenantScope — API surface fixed here, PRE-3 implements

```go
// internal/chquery/scope.go
package chquery

// MustTenantScope wraps a parameterized SELECT/INSERT in a tenant_id WHERE/predicate.
// Panics if ctx has no tenant_id (programmer error — auth middleware should have set it).
// Returns the rewritten query + args ready for clickhouse-go driver.
func MustTenantScope(ctx context.Context, base string, args ...any) (string, []any)
```

PRE-3 implements + adds a CI lint that any file under `internal/query/` or `internal/ingest/` calling `ch.Query(`, `ch.Exec(`, or `conn.QueryRow(` directly (instead of via this helper) fails the build. CH Row Policy enforcement (third layer of ADR-0001 §3.3) lands as part of PRE-3 in the same change.

## Testing

| Layer | What |
|---|---|
| `internal/chquery` | unit — `MustTenantScope` panics without tenant in ctx; injects predicate when tenant present |
| `internal/query/traces` | `-tags=integration` dockertest CH — seed 2 tenants × N spans, hit handler, assert tenant isolation |
| `cmd/query` | none — main is wiring; handler tests cover behavior |
| E2E (Playwright) | new `traces.spec.ts` in SLICE-1: tenant A sees N traces, tenant B sees 0 (cross-tenant reverse test, SLICE-1 AC #8) |

CI changes: backend job unchanged (`go test ./...` already covers the new packages). E2E job's compose-up pulls one more service (negligible startup cost; CH image already cached from SLICE-0).

## Consequences

**Gained:**
- Independent scaling lever from day 1. When traces query starts running 30s scans, gateway ingest path is unaffected.
- Clean boundary: query svc owns CH; gateway svc owns PG. New engineers don't need to learn both.
- Future ingester (SLICE-1 dedicated write-side service for OTel Collector output) follows the same pattern → 3-binary single-image becomes the template.

**Lost / paid:**
- One more compose service. CI startup +5-15s.
- Two PG pools (small memory cost).
- Dockerfile slightly more complex (multi-binary build).

**Reversible:** If somehow Heavy never materializes, fold query handlers back into gateway main — ~2-4h mechanical operation, plus one Caddy line. This is Option B's escape hatch.

## What this ADR does NOT do

- No code changes. PRE-2 is **decision only**.
- Actual `cmd/query/` + `internal/query/` + `internal/chquery/` scaffolding lands in SLICE-1 plan as T1-T3, after PRE-3 implementation (which fills in `MustTenantScope`).
- CH Row Policy DDL belongs to SLICE-1's first migration file.

## Verification (post-implementation)

These are checked at SLICE-1 time, not now:

1. `docker compose -f deploy/docker-compose.yml config` parses with `query` service.
2. `make up` brings `query` to healthy. `curl http://localhost:8081/livez` → 200.
3. Through Caddy: `curl -H "Authorization: Bearer test-key-acme" http://localhost/api/v1/traces` reaches query svc (gateway logs show no hit; query logs show the request).
4. `go build ./...` produces both binaries; CI green.

## Open questions

None blocking. SLICE-1 plan will decide:

- Exact CH client library (`clickhouse-go/v2` vs raw HTTP) — informs `chquery.Connect` signature.

This micro-decision doesn't affect the deploy shape and can be made at implementation time.
