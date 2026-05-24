# ADR-0003: Query API deployment shape — split `cmd/query/` from gateway

- **Status**: Accepted
- **Date**: 2026-05-24
- **Deciders**: @huangbaixun
- **Tracks**: PRE-2 (Slice 1 blocker)
- **Supersedes**: progress.json original recommendation (Option A — extend gateway)
- **Full design**: `docs/specs/2026-05-24-pre-2-query-api-deployment-design.md`

## Context

SLICE-1 introduces the first ClickHouse read path. Before scaffolding handlers, we must decide:

- **A**: extend `cmd/gateway` binary with `/api/v1/traces*` handlers (progress.json's day-1 recommendation, "split later if needed")
- **B**: introduce `cmd/query/` as a second binary from day 1
- **C**: factor query handlers behind a `pkg/queryapi.Router()` factory mounted in gateway, deploy as separate binary only when needed

Two assumptions confirmed during PRE-2 brainstorming (2026-05-24):

1. **Workload is Heavy**: server-side aggregation (topology graph traversal, metric rollups, log scans, long ts-range scans) is known to be coming, not speculative.
2. **Heavy arrives in SLICE-1 / 2**, not deferred to SLICE-4+.

Under Heavy + soon, option A's "split later (1-2d cost)" reasoning inverts: the split cost is paid by the first query that saturates gateway CPU and starves ingest auth. Option C's deferral value collapses when the deferral window is weeks.

## Decision

**Option B** — split `backend/cmd/query/` from day 1. Gateway owns write + admin + auth-ingestion; query owns all CH read endpoints.

Both binaries are produced from a single `backend/Dockerfile` (multi-binary build). Compose picks via `command:`. Caddy routes by path prefix: query-owned prefixes (`/api/v1/traces*`, `/api/v1/logs*`, `/api/v1/services*`, `/api/v1/topology*`) to `query:8081`; everything else under `/api/*` to `gateway:8080`. `handle` (not `handle_path`) preserves prefix end-to-end.

Shared concerns live in `internal/`:

- `internal/auth`, `internal/apikey`, `internal/tenant`, `internal/config`, `internal/httpsrv` — extend existing packages
- `internal/chquery` (**new**) — `MustTenantScope` helper + CH client pool; implemented by PRE-3
- `internal/query` (**new**) — query-only handlers; implemented at SLICE-1 T1-T3

`httpsrv.Run(name, addr, handler)` (small refactor) owns process lifecycle + slog setup. Each main owns middleware mounting + routes. Both mains share the same generic middleware block (RequestID, Recoverer, auth).

## Consequences

**Gained:**
- Independent scaling lever from day 1. Heavy query saturating CH does not impact gateway's auth path.
- Clean ownership: gateway↔PG, query↔CH. Future ingester (SLICE-1 separate write-side service) follows the same multi-binary single-image template.
- `cmd/query/` becomes the natural host for future read endpoints (logs, services, topology) — no re-org needed per slice.

**Paid:**
- One additional compose service. CI startup +5-15s.
- Two PG connection pools (small RAM cost). Failure-isolation upside.
- Dockerfile slightly more complex (two `go build` invocations, two binaries copied).
- Both mains run the same generic-middleware boilerplate (4-5 lines). Acceptable; can be de-duped by a `httpsrv.NewMux()` helper if it becomes annoying.

**Reversible**: if Heavy never materializes, fold query handlers back into gateway main (~2-4h mechanical) + flip one Caddy block. Escape hatch documented.

**Locked decision**: metering only on the write path. Query requests do not write `metering_events` (page refresh otherwise burns quota = bad UX). Read-side billing revisited post-MVP.

**Locked API surface**: `chquery.MustTenantScope(ctx, base, args...) (string, []any)` — PRE-3 implements + adds a build/lint check that any file under `internal/query/` or `internal/ingest/` calling `ch.Query(`/`ch.Exec(`/`conn.QueryRow(` directly fails the build. CH Row Policy (3rd layer of ADR-0001 §3.3) lands with PRE-3 in the same change.

## What this ADR does NOT do

- No code changes. PRE-2 is decision-only.
- `cmd/query/`, `internal/query/`, `internal/chquery/` scaffolding lands in SLICE-1 plan as T1-T3, sequenced after PRE-3 implementation.
- CH Row Policy DDL belongs to SLICE-1's first migration file.

## Verification (post-implementation, at SLICE-1 time)

1. `docker compose -f deploy/docker-compose.yml config` parses with the `query` service entry.
2. `make up` brings `query` to `healthy`. `curl http://localhost:8081/livez` → 200.
3. Through Caddy: `curl -H "Authorization: Bearer test-key-acme" http://localhost/api/v1/traces` reaches query svc (gateway logs show no hit; query logs show the request).
4. `go build ./...` produces both `gateway` and `query` binaries.
5. CI green on the new `cmd/query` package.

These verification steps are tracked by SLICE-1 plan, not by this ADR.

## Forward references

- PRE-3 (next): implements `internal/chquery.MustTenantScope` + CH Row Policy DDL + cross-tenant reverse E2E. Blocks SLICE-1 T4+ (any code path that reads/writes CH).
- SLICE-1 plan (after PRE-3): scaffolds `cmd/query/`, `internal/query/traces`, the first ch-migration file, and the Caddy/compose changes from this ADR.
