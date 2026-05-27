---
date: 2026-05-27
topic: slice-3-services-topology-design
type: feature
status: proposed
features: [SLICE-3]
---

# SLICE-3 design — Services + Topology

## Context

SLICE-1 (closed 2026-05-26, 10/10 ACs) shipped the trace vertical: `cmd/ingester` → CH `traces_v1` (Row Policy) → `cmd/query` → Vue `/traces` page. SLICE-2 (closed 2026-05-26, 10/10 ACs) shipped the log vertical on the same pattern, plus closed drift D4 (Caddy is now the sole API ingress).

SLICE-3 turns the in-place trace data into an **interactive topology** and the foundation for service-centric navigation. The architectural skeleton is established; SLICE-3 introduces a fourth binary (`cmd/topo-engine`) — a background aggregator that periodically derives service-to-service edges and per-service RED metrics from `traces_v1`, plus three new query-side endpoints and four new frontend pages.

Brainstormed 2026-05-27 (this session). **Seven design choices locked** via per-decision multi-choice picks during brainstorming:

1. **Edge derivation** = 1min background job → `topology_edges_v1` (per ADR-0001 §5). Not query-time aggregation (latency); not CH MV (debug difficulty).
2. **UI scope** = full ADR-0001 scope: Overview cards + global Topology page + Service detail page (Signals + Dependencies tabs, 4 placeholder tabs) + `/traces/:id` Service Map subtab body. **⌘K injection de-scoped** to SLICE-3.5 / SLICE-5 — the cmd palette skeleton ADR-0001 §4.4 promised in Slice 0 was YAGNI'd during execution.
3. **Graph rendering** = `d3-force` (~30KB gz, simulation only) + hand-rolled Vue SVG. Consistent with SLICE-1's hand-rolled-waterfall philosophy.
4. **External nodes** = walked from `peer.service` / `db.system` / `messaging.system` / `http.host` attributes via `COALESCE`. Topology can show "checkout → redis" and "order → stripe.com", not only OTLP-instrumented services.
5. **Time window UX** = fixed buckets `15m / 1h / 6h / 24h` (per ADR-0001) with `?window=` URL param. Aligned with `topology_edges_v1` 1min bucket granularity.
6. **Aggregation tables** = two tables from one `cmd/topo-engine` 1min job: `topology_edges_v1` (per-pair edge stats) + `service_stats_v1` (per-service RED, split by `span_kind`).
7. **`/traces/:id` Service Map** = **client-side derivation** from already-loaded spans (`TraceDetail.vue` fetches spans once for the waterfall; Service Map subtab is a Vue `computed` on the same data). Zero backend changes, zero new endpoint, zero duplicate fetch.

**Idempotency model** chosen for both new tables: `ReplacingMergeTree` + queries with `FINAL`. Justification: topo-engine re-runs cleanly after crash without PG-tracked exactly-once complexity; queries get post-merge view via `FINAL`; the tables are small (medium tenant ~200k rows / 24h) so `FINAL` cost is acceptable.

## §1 Topology — 4 binaries + 2 new CH tables

```
                                  ┌─────────────────────────┐
                                  │   docker-compose stack  │
SDK / hot-r.o.d.                  │                         │
     │ OTLP                       │  ┌──────────┐           │
     │ Bearer <api-key>           │  │ Postgres │◄──┐       │
     ▼                            │  └──────────┘   │       │
┌──────────────┐                  │       ▲         │       │
│  ingester    │ ── trace batch ──┼──► traces_v1    │       │
│ 4317/4318    │                  │  ┌──────────┐   │       │
└──────────────┘                  │  │    CH    │   │       │
┌──────────────┐                  │  │          │   │       │
│ log-ingester │ ── log batch ────┼──► logs_v1  │   │       │
│ 4327/4328    │                  │  │          │   │       │
└──────────────┘                  │  ├──────────┤   │       │
                                  │  │NEW SLICE-3│  │       │
┌──────────────┐   1min ticker    │  │ topology_│   │       │
│ topo-engine  │ ◄────────────────┼──┤ edges_v1 │   │       │
│   :8084 adm  │   reads traces_v1│  │service_  │   │       │
│  (NEW)       │   writes 2 tables│  │ stats_v1 │   │       │
└──────────────┘                  │  └────┬─────┘   │       │
                                  │       │ chquery │       │
                                  │  ┌────┴────┐  ┌─┴──────┐│
                                  │  │ gateway │  │ query  ││
                                  │  │  :8080  │  │ :8081  ││
                                  │  └─────────┘  └────────┘│
                                  └──────────────────┼──────┘
Browser ──► Caddy :443 ──┬─ /api/v1/traces*        → query:8081
                         ├─ /api/v1/logs*          → query:8081
                         ├─ /api/v1/services*  NEW → query:8081
                         ├─ /api/v1/topology*  NEW → query:8081
                         └─ /api/*                 → gateway:8080
```

**Four binaries** (single shared image, multi-target; new binary in **bold**):

| binary | port | role | deps |
|---|---|---|---|
| `cmd/gateway` (unchanged) | 8080 | admin / metering / health | PG |
| `cmd/query` (unchanged code; +2 route files) | 8081 | CH read path; gains `/api/v1/services*` + `/api/v1/topology*` | PG (auth) + CH (data) |
| `cmd/ingester` (unchanged) | 4317 / 4318 / 8082 | OTLP **trace** receiver | PG + CH |
| `cmd/log-ingester` (unchanged) | 4327 / 4328 / 8083 | OTLP **log** receiver | PG + CH |
| **`cmd/topo-engine`** (new) | **:8084 admin** | **1min ticker → reads `traces_v1` → writes `topology_edges_v1` + `service_stats_v1`** | PG + CH |

Port rationale: `:8084` continues the admin-port sequence (`8082` ingester / `8083` log-ingester / **`8084` topo-engine**). Host port env var: `TOPO_ENGINE_ADMIN_HOST_PORT` added to `deploy/.env.example` (default 8084).

**Caddy** routing rules added (`deploy/Caddyfile`) — two new `handle` blocks listed before the catch-all `/api/*` per existing first-match-wins pattern:

```caddyfile
handle /api/v1/services* {
    uri strip_prefix /api
    reverse_proxy query:8081
}
handle /api/v1/topology* {
    uri strip_prefix /api
    reverse_proxy query:8081
}
```

Zero nginx changes (D4 closed in SLICE-2; this is exactly the "future routes are one-line Caddy adds" pattern SLICE-2 spec §7 promised).

## §2 Tenant trust — topo-engine is an internal service

`cmd/topo-engine` is **the only binary that accepts no external Bearer**. (`cmd/ingester` and `cmd/log-ingester` are Bearer-on-receiver; `cmd/gateway` and `cmd/query` are Bearer-on-request.) Its tenant context is derived **from data**, not from a request, and that requires care:

**Tenant discovery** — start of each 1min tick:

```go
// internal/topoengine/tenants.go
func (e *Engine) activeTenants(adminCtx context.Context, since time.Time) ([]tenant.ID, error) {
    // adminCtx has NO tenant_id. MustTenantScope would reject this query.
    // Use the new chquery.AdminConn: bypasses MustTenantScope but is whitelisted.
    rows, err := e.adminCH.AdminQuery(adminCtx,
        chquery.AdminListTenants, // pre-registered SQL: SELECT DISTINCT tenant_id FROM traces_v1 WHERE ts >= ?
        since,
    )
    // ...
}
```

This requires a **small extension** to `chquery` (added in T0 of the plan):

- New `chquery.AdminConn` type — wraps `driver.Conn` directly, bypasses `MustTenantScope`.
- New `AdminQuery(ctx, kind, args...)` method — `kind` is an enum (`AdminListTenants`, `AdminMaxBucket`, …), each backed by a SQL constant. **No free-form SQL strings.**
- `make lint-ch` adds a rule: `AdminConn` may only be constructed under `internal/topoengine/`. Forbidden anywhere else. CI fails the build.

**Per-tenant processing** — inside the tick:

```go
for _, tid := range tenants {
    tctx := auth.WithTenant(adminCtx, tid)
    if err := e.runBucketForTenant(tctx, bucket); err != nil {
        topo_engine_tenant_failed_total.WithLabelValues(tid.String()).Inc()
        slog.Error("topo-engine tenant failed", "tenant", tid, "err", err)
        continue // single tenant failure must not abort the tick
    }
}
```

`runBucketForTenant` uses `tctx` with `chquery.Conn` (not `AdminConn`); the standard tenant scoping enforces three layers:

| Layer | Mechanism | Where |
|---|---|---|
| L1 | SQL `WHERE tenant_id = ?` predicate (explicit) | §4 edges.go / services.go |
| L2 | CH Row Policies on both new tables | §3 migrations |
| L3 | `chquery.Conn` auto-injects `custom_tenant_id` session setting | SLICE-1 PRE-3 (unchanged) |

A reverse cross-tenant write test (`TestTopoEngine_CannotWriteEdgeAcrossTenant`) seeds tenant A spans, attempts to run `RunOnce` with tenant B's ctx — assert: no rows in either new table carry tenant A's data (Layer 1 SQL filter blocks it; even with a bug, Layer 2 Row Policy would reject the INSERT).

## §3 CH schemas — two new aggregation tables

### `topology_edges_v1`

Migration: `backend/ch-migrations/20260527120000_create_topology_edges_v1.sql`.

```sql
CREATE TABLE IF NOT EXISTS topology_edges_v1 (
    tenant_id      LowCardinality(String),
    ts_bucket      DateTime CODEC(Delta, ZSTD(1)),    -- 1min aligned
    caller_service LowCardinality(String),
    caller_kind    LowCardinality(String),            -- always 'service'
    callee_service LowCardinality(String),
    callee_kind    LowCardinality(String),            -- 'service' | 'external'
    calls          UInt64 CODEC(T64, LZ4),
    errors         UInt64 CODEC(T64, LZ4),
    p95_duration   UInt64 CODEC(T64, LZ4)             -- nanoseconds
) ENGINE = ReplacingMergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts_bucket))
ORDER BY (tenant_id, ts_bucket, caller_service, callee_service, callee_kind)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_topology_edges_v1 ON topology_edges_v1
    USING tenant_id = getSetting('custom_tenant_id') TO openaiops;
```

### `service_stats_v1`

Migration: `backend/ch-migrations/20260527120100_create_service_stats_v1.sql`.

```sql
CREATE TABLE IF NOT EXISTS service_stats_v1 (
    tenant_id     LowCardinality(String),
    ts_bucket     DateTime CODEC(Delta, ZSTD(1)),     -- 1min aligned
    service       LowCardinality(String),
    span_kind     LowCardinality(String),             -- Server | Client | Internal | Producer | Consumer
    calls         UInt64 CODEC(T64, LZ4),
    errors        UInt64 CODEC(T64, LZ4),
    p95_duration  UInt64 CODEC(T64, LZ4)              -- nanoseconds
) ENGINE = ReplacingMergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts_bucket))
ORDER BY (tenant_id, ts_bucket, service, span_kind)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_service_stats_v1 ON service_stats_v1
    USING tenant_id = getSetting('custom_tenant_id') TO openaiops;
```

### Schema choices

- **`ReplacingMergeTree`** — topo-engine re-runs cleanly: same `(tenant_id, ts_bucket, caller, callee, callee_kind)` written twice → background MERGE keeps the latest version. Queries use `FROM … FINAL` for the post-merge view; we **do not** schedule `OPTIMIZE … FINAL` (too costly). The aggregation SQL (Pass A and Pass B) is deterministic — same input rows yield same output — so the "latest version" is the correct version regardless of which run wrote it. ([cite: §B brainstorming recommendation])
- **`ORDER BY (tenant_id, ts_bucket, …)`** — `ts_bucket` second (not `service` like `traces_v1`) because the primary access pattern is "all-services in a time window," not "one-service over time."
- **`PARTITION BY (tenant_id, toYYYYMMDD(ts_bucket))`** — same template as `traces_v1` / `logs_v1`. Because these tables are aggregated (1min granularity), partition pressure is far below the trace/log volume — at 1min × ~50 services × 24h ≈ 72k rows per tenant per day, the 1000-partition CH soft ceiling is irrelevant here.
- **`callee_kind` in ORDER BY** — a single `(caller, callee)` pair may legitimately appear twice (once as `service`, once as `external` — e.g., `checkout → redis` could exist both as an external db call AND as an instrumented service call if a tenant runs an OTLP-instrumented redis wrapper). The kind disambiguates.
- **No skip indexes** — the tables are small enough that primary key scan is fast. Add if production query patterns demand.

Row Policy templates come verbatim from `backend/ch-migrations/README.md` (added during PRE-3 T4).

## §4 `cmd/topo-engine` — internal aggregation loop

### Code layout

```
backend/
  cmd/topo-engine/main.go              # config → PG → CH → ticker → loop
  internal/topoengine/
    engine.go                          # Engine struct + RunOnce(ctx, bucket) + Catchup(ctx)
    tenants.go                         # active tenants via chquery.AdminConn
    edges.go                           # Pass A: edge SQL + Exec
    services.go                        # Pass B: per-service SQL + Exec
    state.go                           # max(ts_bucket) per tenant lookup
    metrics.go                         # Prom counters/histograms
    engine_test.go                     # unit (bucket math, COALESCE order)
    cross_tenant_test.go               # integration (6 sub-assertions; SLICE-3 signature test)
    catchup_test.go                    # integration (cold start replay)
    write_isolation_test.go            # integration (cross-tenant WRITE attempt)
  internal/chquery/
    admin.go                           # NEW: AdminConn + AdminQuery + whitelisted SQL constants
    admin_test.go                      # unit
```

### Bucket discipline

`topo-engine` **never touches the current bucket**. At tick `T`, it processes `[T-2min, T-1min)` — the latest **closed** bucket. This guarantees no in-flight ingest writes can land in a bucket while it's being aggregated.

```go
// internal/topoengine/engine.go
func closedBucketAt(t time.Time) time.Time {
    return t.Truncate(time.Minute).Add(-time.Minute)
}
```

`main.go` runs the loop:

```go
ticker := time.NewTicker(cfg.TickInterval) // default 1m
for {
    select {
    case <-ctx.Done(): return
    case t := <-ticker.C:
        bucket := closedBucketAt(t)
        if err := engine.RunOnce(ctx, bucket); err != nil {
            slog.Error("topo-engine tick failed", "bucket", bucket, "err", err)
            topo_engine_tick_failed_total.Inc()
        }
        topo_engine_bucket_lag_seconds.Set(time.Since(bucket).Seconds())
    }
}
```

### Cold-start catchup

On startup, run once: discover each tenant's `max(ts_bucket)` in `topology_edges_v1` (via `chquery.AdminConn`), then replay every missing bucket up to `closedBucketAt(now)`. Capped at `TOPO_CATCHUP_MAX` (default 1h) — if the gap is larger, we accept the loss rather than burning hours on cold catch-up.

```go
func (e *Engine) Catchup(ctx context.Context) error {
    tenants, _ := e.activeTenants(ctx, time.Now().Add(-cfg.CatchupMax))
    for _, tid := range tenants {
        tctx := auth.WithTenant(ctx, tid)
        last := e.lastCompletedBucket(tctx) // SELECT max(ts_bucket) FROM topology_edges_v1 FINAL
        if last.IsZero() || time.Since(last) > cfg.CatchupMax {
            last = time.Now().Add(-cfg.CatchupMax)
        }
        for b := last.Add(time.Minute); b.Before(closedBucketAt(time.Now())); b = b.Add(time.Minute) {
            if err := e.runBucketForTenant(tctx, b); err != nil { return err }
        }
    }
    return nil
}
```

`Catchup` runs on a separate goroutine so the ticker isn't blocked. After `Catchup` returns, the ticker is responsible for keeping pace.

### Multi-tenant concurrency

Within a single bucket, tenants are processed concurrently with `errgroup` limited to `TOPO_TENANT_CONCURRENCY` (default 4). CH SELF JOIN is not cheap; 4 in-flight tenants is enough to saturate a single CH replica without triggering MERGE backpressure.

### Pass A — edge SQL

```sql
INSERT INTO topology_edges_v1 (
    tenant_id, ts_bucket,
    caller_service, caller_kind,
    callee_service, callee_kind,
    calls, errors, p95_duration
)
SELECT
    /* tenant_id */ ?,
    /* ts_bucket */ toStartOfMinute(b.ts),
    -- internal edge: caller is the parent's service. external edge: caller is the Client span's own service.
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, a.service,
        b.service
    ) AS caller_service,
    'service' AS caller_kind,
    -- callee precedence: real OTLP service first, then peer attributes in trust order, then sentinel.
    -- Note: for internal edges b.service is always non-empty (real OTLP child); COALESCE short-circuits there.
    --       for external edges b.service is the client-side service (same as caller), so we must skip it and
    --       fall through to peer attributes; the multiIf below handles the disambiguation.
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, b.service,           -- internal: callee is child's service
        coalesce(                                                              -- external: derive from peer attrs
            nullIf(b.attributes['peer.service'], ''),
            nullIf(b.attributes['db.system'], ''),
            nullIf(b.attributes['messaging.system'], ''),
            nullIf(b.attributes['http.host'], ''),
            'unknown-external'
        )
    ) AS callee_service,
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, 'service',
        'external'
    ) AS callee_kind,
    count() AS calls,
    countIf(b.status = 'Error') AS errors,
    toUInt64(quantile(0.95)(b.duration)) AS p95_duration
FROM traces_v1 b
LEFT JOIN traces_v1 a
    ON  a.tenant_id = b.tenant_id
    AND a.trace_id  = b.trace_id
    AND a.span_id   = b.parent_span_id
WHERE b.tenant_id = ?
  AND b.ts >= ? AND b.ts < ?
  AND (
    -- internal edge: parent exists and is a different service
    (a.service IS NOT NULL AND a.service != b.service)
    OR
    -- external edge: Client span with parent, parent either missing (NULL) or same-service, and has a peer attr
    (b.span_kind = 'Client' AND b.parent_span_id != ''
     AND (a.service IS NULL OR a.service = b.service)
     AND (
       has(b.attributes, 'peer.service') OR
       has(b.attributes, 'db.system') OR
       has(b.attributes, 'messaging.system') OR
       has(b.attributes, 'http.host')
     ))
  )
GROUP BY caller_service, callee_service, callee_kind, ts_bucket
SETTINGS join_use_nulls = 1   -- required: without this, LEFT JOIN unmatched rows return '' (LC default), not NULL, and IS NULL checks would silently fail
```

Five non-obvious details:

1. **`SETTINGS join_use_nulls = 1`** is required, not optional. CH's LEFT JOIN default returns the column's *default value* for unmatched rows (`''` for `LowCardinality(String)`), not `NULL`. Without this setting, `a.service IS NULL` never fires and the external-edge clause silently misses "Client span whose parent wasn't ingested" — an alarming amount of demo external edges would disappear.
2. **Caller / callee precedence via `multiIf` (not `COALESCE`)**: internal vs external edges have *different* sources for both caller and callee. Internal edge: caller = `a.service` (parent), callee = `b.service` (child). External edge: caller = `b.service` (the Client span itself is the service making the call), callee = derived from peer attributes. A single `COALESCE` cannot express this — the multiIf branches on the same `(a.service IS NOT NULL AND a.service != b.service)` predicate used in WHERE.
3. **Peer attribute trust order** (in external branch): `peer.service` (most explicit OTLP convention) → `db.system` → `messaging.system` → `http.host` → `'unknown-external'` sentinel. Each wrapped in `nullIf(..., '')` because `attributes` is `Map(LC, String)` where missing keys also return `''`, not NULL — same gotcha as JOIN.
4. **External edge dedup guard**: `(a.service IS NULL OR a.service = b.service)` in WHERE prevents a real internal edge AND a peer-attribute-derived edge from being counted twice when both happen to exist for the same span.
5. **`status = 'Error'`** is the SLICE-1 enum-style status, not raw OTLP `status_code`. Confirmed by OQ-1 deferred to T2 implementation.

### Pass B — per-service SQL

```sql
INSERT INTO service_stats_v1 (tenant_id, ts_bucket, service, span_kind, calls, errors, p95_duration)
SELECT
    /* tenant_id */ ?,
    toStartOfMinute(ts),
    service,
    span_kind,
    count(),
    countIf(status = 'Error'),
    toUInt64(quantile(0.95)(duration))
FROM traces_v1
WHERE tenant_id = ?
  AND ts >= ? AND ts < ?
GROUP BY service, span_kind, ts_bucket
```

### Counters (`topo_engine_*`)

- `topo_engine_tick_total{outcome}` — `outcome ∈ {ok, partial, failed}`
- `topo_engine_tick_failed_total`
- `topo_engine_tenant_failed_total{tenant_id}`
- `topo_engine_tenants_processed_total`
- `topo_engine_edges_written_total{tenant_id}`
- `topo_engine_services_written_total{tenant_id}`
- `topo_engine_bucket_lag_seconds` (gauge — `now - last_completed_bucket`; alerts on > 5min)
- `topo_engine_pass_duration_seconds{pass}` — `pass ∈ {edges, services}` (histogram)

All counters on `:8084/metrics` (Prom format), `/healthz` returns 200 once `Catchup` completes and at least one tick has run.

## §5 Query API — 3 new endpoints on `cmd/query` :8081

**Code layout:**

```
backend/internal/query/
  topology_handler.go     # GET /v1/topology
  topology_repo.go        # reads topology_edges_v1 FINAL + service_stats_v1 FINAL
  services_handler.go     # GET /v1/services + GET /v1/services/{name}
  services_repo.go        # reads service_stats_v1 FINAL + topology_edges_v1 FINAL
  topology_repo_test.go   # integration
  services_repo_test.go   # integration
  router.go               # MODIFY — register 3 new routes (auth.Middleware unchanged)
```

### `GET /api/v1/services`

**Query params:**

| param | type | default | validation |
|---|---|---|---|
| `window` | enum `15m / 1h / 6h / 24h` | `1h` | handler whitelist → CH `INTERVAL N {MINUTE,HOUR}` |
| `limit` | int | `100`, max `500` | handler cap |
| `sort` | enum `calls / errors / p95` | `calls` | handler whitelist (protects template substitution) |

**Repo SQL:**

```sql
SELECT
    service,
    sumIf(calls, span_kind = 'Server')             AS inbound_calls,
    sumIf(errors, span_kind = 'Server')            AS inbound_errors,
    maxIf(p95_duration, span_kind = 'Server')      AS inbound_p95,
    sumIf(calls, span_kind = 'Client')             AS outbound_calls
FROM service_stats_v1 FINAL
WHERE tenant_id = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY service
HAVING inbound_calls > 0 OR outbound_calls > 0
ORDER BY inbound_calls DESC
LIMIT ?
```

`maxIf(p95)` returns "worst 1min bucket's p95 in the window" — an approximation, not the true window-quantile. The true quantile requires `quantileTDigestState` in the topo-engine writes (added in SLICE-4 alongside metrics aggregation; see §10).

**Response:**

```json
{
  "window": "1h",
  "items": [
    {
      "service": "checkout",
      "inbound_calls": 12450,
      "inbound_errors": 23,
      "inbound_error_rate": 0.00185,
      "inbound_p95_ms": 47.3,
      "outbound_calls": 9870
    }
  ]
}
```

### `GET /api/v1/services/{name}`

Combines per-service RED stats + inbound/outbound dependencies. **Single endpoint, not `/services/{name}/dependencies`** — front end loads the whole detail page once, no second hop.

**Repo SQL:**

```sql
-- self stats (same shape as /services list, filtered)
SELECT span_kind, sum(calls), sum(errors), max(p95_duration)
FROM service_stats_v1 FINAL
WHERE tenant_id = ? AND service = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY span_kind;

-- inbound dependencies (who calls me)
SELECT caller_service AS peer, 'service' AS peer_kind,
       sum(calls), sum(errors), max(p95_duration)
FROM topology_edges_v1 FINAL
WHERE tenant_id = ? AND callee_service = ? AND callee_kind = 'service'
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY caller_service
ORDER BY sum(calls) DESC LIMIT 50;

-- outbound dependencies (who I call)
SELECT callee_service AS peer, callee_kind AS peer_kind,
       sum(calls), sum(errors), max(p95_duration)
FROM topology_edges_v1 FINAL
WHERE tenant_id = ? AND caller_service = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY callee_service, callee_kind
ORDER BY sum(calls) DESC LIMIT 50;
```

**Response:**

```json
{
  "service": "checkout",
  "window": "1h",
  "stats": {
    "inbound":  { "calls": 12450, "errors": 23, "error_rate": 0.00185, "p95_ms": 47.3 },
    "outbound": { "calls": 9870 }
  },
  "dependencies": {
    "inbound":  [{ "peer": "frontend", "peer_kind": "service", "calls": 12450, "errors": 23, "p95_ms": 47.3 }],
    "outbound": [
      { "peer": "payment", "peer_kind": "service",  "calls": 6230, "errors": 12, "p95_ms": 38.9 },
      { "peer": "redis",   "peer_kind": "external", "calls": 3640, "errors": 0,  "p95_ms": 2.1 }
    ]
  }
}
```

**404** if the named service appears in neither `service_stats_v1` nor as a participant in any edge within the window.

### `GET /api/v1/topology`

**Query params:** `window` (same enum); `node_limit` int default `100`, max `300`.

**Repo SQL:**

```sql
WITH top_services AS (
    SELECT service FROM (
        SELECT service, sum(calls) AS c FROM service_stats_v1 FINAL
        WHERE tenant_id = ? AND span_kind = 'Server'
          AND ts_bucket >= now() - INTERVAL ? MINUTE
        GROUP BY service ORDER BY c DESC LIMIT ?
    )
)
SELECT
    caller_service, caller_kind,
    callee_service, callee_kind,
    sum(calls) AS calls,
    sum(errors) AS errors,
    max(p95_duration) AS p95_duration
FROM topology_edges_v1 FINAL
WHERE tenant_id = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
  AND caller_service IN top_services
  AND (callee_kind = 'external' OR callee_service IN top_services)
GROUP BY caller_service, caller_kind, callee_service, callee_kind
```

Top-N truncation by inbound `calls` (Server kind only — Client outbound calls aren't a meaningful "service is busy" signal). External nodes are **never** truncated — they're leaves and richer demo signal than top-N filtering would suggest.

**Response:**

```json
{
  "window": "1h",
  "nodes": [
    { "service": "frontend", "kind": "service",  "calls": 24800, "errors": 35, "p95_ms": 12.1 },
    { "service": "checkout", "kind": "service",  "calls": 12450, "errors": 23, "p95_ms": 47.3 },
    { "service": "redis",    "kind": "external", "calls": 18200, "errors": 0,  "p95_ms": 2.1 }
  ],
  "edges": [
    { "caller": "frontend", "callee": "checkout", "callee_kind": "service",  "calls": 12450, "errors": 23, "p95_ms": 47.3 },
    { "caller": "checkout", "callee": "redis",    "callee_kind": "external", "calls": 3640,  "errors": 0,  "p95_ms": 2.1 }
  ]
}
```

Empty window returns `200` + `{nodes: [], edges: []}` (empty graph is a valid state, not an error).

### `chquery` / lint

All three repos use `cn.Query(ctx, sql, args...)`. `chquery.MustTenantScope` sees `tenant_id = ?` in the SQL and passes the check (Layer 1). Row Policy backs it up (Layer 2). `make lint-ch` automatically covers the new files (rule pre-existing from PRE-3: any bare `clickhouse-go` import outside `internal/chquery` fails CI).

### Error semantics

| Path | Condition | Status |
|---|---|---|
| any | missing `Authorization` / wrong Bearer | 401 (from `auth.Middleware`, unchanged) |
| any | `window` not in whitelist | 400 |
| `/services/{name}` | no stats and no edges in window | 404 |
| `/topology` | no services / edges in window | 200 + empty arrays |

## §6 Frontend — 4 new pages + 1 shared graph component

### Files

```
frontend/src/
  views/
    Overview/
      OverviewPage.vue              # /overview — service-card grid
      ServiceCard.vue               # RED color ring + call volume + error rate
    Services/
      ServiceDetail.vue             # /services/:name — NTabs shell (6 tabs)
      SignalsTab.vue                # tab 1 — RED metric strip + quick-links to /traces, /logs
      DependenciesTab.vue           # tab 2 — inbound/outbound tables + mini ServiceGraph
      ComingSoonTab.vue             # tabs 3-6 placeholder (NEmpty, matches SLICE-1 Service Map placeholder)
    Topology/
      TopologyPage.vue              # /topology — global force-directed graph
    Traces/
      TraceDetail.vue               # MODIFY — Service Map subtab body lights up
      ServiceMapPanel.vue           # NEW — client-side derivation from in-memory spans
  components/
    ServiceGraph/
      ServiceGraph.vue              # shared 3-way reuse
      useForceSimulation.ts         # d3-force composable
      types.ts                      # GraphNode / GraphEdge
    SideBar.vue                     # MODIFY — overview / services / topology entries enabled
    TimeWindowPicker.vue            # NEW — NRadioGroup 15m/1h/6h/24h, URL-synced
  composables/
    useTimeWindow.ts                # URL ↔ ?window= bidirectional binding (suppressNextWatch pattern)
    useTopologyApi.ts               # GET /api/v1/topology
    useServicesApi.ts               # GET /api/v1/services + /services/:name
  router/index.ts                   # MODIFY — register 3 new routes
  i18n/locales/{zh-CN,en-US}.ts     # MODIFY — overview / services / topology / tab labels
```

### Shared `<ServiceGraph>` component

Three consumers, all use the same component:

| Consumer | Source | Typical node count |
|---|---|---|
| `/topology` | `GET /api/v1/topology?window=1h` | ≤ 100 |
| `/services/:name` Dependencies tab | derived from `/services/{name}` response | ≤ 30 |
| `/traces/:id` Service Map subtab | **client-side derived** from in-memory spans | typically ≤ 10 |

Force-directed simulation uses `d3-force` only (no `d3-selection`, no `d3-zoom`):

```ts
import { forceSimulation, forceManyBody, forceLink, forceCenter, forceCollide } from 'd3-force'

export function useForceSimulation(nodes, edges, opts) {
  const positions = ref<Record<string, {x, y}>>({})
  let sim
  watchEffect(() => {
    sim?.stop()
    sim = forceSimulation(nodes.value)
      .force('charge', forceManyBody().strength(-300))
      .force('link', forceLink(edges.value).id(d => d.service).distance(80))
      .force('center', forceCenter(opts.width / 2, opts.height / 2))
      .force('collide', forceCollide(d => radiusFor(d) + 4))
      .on('tick', () => {
        positions.value = Object.fromEntries(
          nodes.value.map(n => [n.service, { x: n.x!, y: n.y! }]))
      })
  })
  return { positions, restart: () => sim?.alpha(1).restart() }
}
```

Rendering uses plain Vue + SVG. Visual encoding rules:

| Dimension | Encoding |
|---|---|
| Node radius | `sqrt(calls)`, min 12 / max 40 |
| Node fill | `error_rate` 0→1 mapped to green→yellow→red ring |
| Node stroke | `kind=external` → dashed grey; `service` → solid primary |
| Edge stroke width | `log(calls)`, min 1 / max 6 |
| Edge color | `errors/calls` on the same color ring as node fill |

Events:

- `node-click` — `/topology` navigates to `/services/:name`; `/services/:name` Dep tab navigates to the peer service; `/traces/:id` ServiceMap scrolls the Waterfall to the first span of that service (`?focus_service=X` URL param).

### `<TimeWindowPicker>` and URL state

`?window=` is the single source of truth across `/overview`, `/services/:name`, `/topology`. The composable:

```ts
export function useTimeWindow(defaultWindow = '1h') {
  const route = useRoute()
  const router = useRouter()
  const windowVal = ref(validWindow(route.query.window) ?? defaultWindow)
  let suppressNextWatch = false

  const apply = (next: string) => {
    if (next === windowVal.value) return
    suppressNextWatch = true
    router.replace({ query: { ...route.query, window: next } })
    windowVal.value = next
  }
  watch(() => route.query.window, q => {
    if (suppressNextWatch) { suppressNextWatch = false; return }
    windowVal.value = validWindow(q) ?? defaultWindow
  })
  return { windowVal, apply }
}
```

**`suppressNextWatch` is load-bearing** — directly reuses the SLICE-2 LogsPanel pattern that closed bug T9/T10 (apply → router.replace → watch → reload double-fetch loop). Adding a unit test for this is mandatory; see §8.

### Service Map subtab — `ServiceMapPanel.vue`

Replaces the SLICE-1 NEmpty placeholder in `TraceDetail.vue`. Pure client-side computation from already-loaded spans:

```vue
<script setup lang="ts">
const { spans } = defineProps<{ spans: Span[] }>()
const graph = computed(() => {
  const spanIndex = new Map(spans.map(s => [s.span_id, s]))
  const nodeMap = new Map<string, GraphNode>()
  const edges: GraphEdge[] = []
  for (const s of spans) {
    if (!nodeMap.has(s.service)) {
      nodeMap.set(s.service, { service: s.service, kind: 'service', calls: 0 })
    }
    nodeMap.get(s.service)!.calls++
    if (!s.parent_span_id) continue
    const p = spanIndex.get(s.parent_span_id)
    if (p && p.service !== s.service) {
      edges.push({
        caller: p.service, callee: s.service, callee_kind: 'service',
        calls: 1, errors: s.status === 'Error' ? 1 : 0,
        p95_duration: s.duration,
      })
    }
  }
  return { nodes: [...nodeMap.values()], edges }
})
</script>
<template>
  <ServiceGraph :nodes="graph.nodes" :edges="graph.edges" @node-click="$emit('node-click', $event)" />
</template>
```

Per-trace view **only renders `service` nodes** — peer-attribute external edge derivation is unstable within a single trace and would mislead. External nodes appear only in the global `/topology`.

### Router + SideBar

```ts
{ path: '/overview',       component: OverviewPage,  meta: { requiresAuth: true } },
{ path: '/services/:name', component: ServiceDetail, meta: { requiresAuth: true } },
{ path: '/topology',       component: TopologyPage,  meta: { requiresAuth: true } },
```

SideBar — the three `to: null` entries become real paths:

```ts
{ key: 'overview', label: 'nav.overview', to: '/overview' },
{ key: 'services', label: 'nav.services', to: '/overview' },  // shares the card grid
{ key: 'topology', label: 'nav.topology', to: '/topology' },
```

The `services` sidebar item routing to `/overview` is intentional — the cards ARE the services list. A separate `/services` table view is deferred (see §10).

## §7 Caddy / nginx (D4 stays closed)

Only Caddy changes — two new `handle` blocks (§1). **Zero changes to `frontend/nginx.conf`** (SLICE-2 closed drift D4, removing all `/api/*` blocks; the SPA-loading container stays static-only). This is the exact "future routes are one-line Caddy adds" pattern SLICE-2 spec §7 promised — SLICE-3 is the first slice to prove the promise holds.

## §8 Cross-tenant test — SLICE-3 signature integration test

`backend/internal/topoengine/cross_tenant_test.go`, build tag `integration`, runs against dockertest CH + PG.

```go
//go:build integration

func TestSlice3_CrossTenantTopology(t *testing.T) {
    ctx, deps := chtest.Setup(t)
    tidA := tenant.MustParse("aaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
    tidB := tenant.MustParse("bbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
    keyA := seedAPIKey(t, deps.PG, tidA, "test-key-acme")
    keyB := seedAPIKey(t, deps.PG, tidB, "test-key-beta")

    // --- HOIST setup outside t.Run (SLICE-2 T11 lesson) ---
    bucket := time.Now().Truncate(time.Minute).Add(-2 * time.Minute)
    seedSpansForTenant(t, deps.CH, tidA, bucket,
        []spanSpec{ /* frontend → checkout → payment, checkout → redis */ })
    seedSpansForTenant(t, deps.CH, tidB, bucket,
        []spanSpec{ /* mobile → orders */ })

    eng := topoengine.New(deps.CH, deps.PG)
    require.NoError(t, eng.RunOnce(auth.WithTenant(ctx, tidA), bucket))
    require.NoError(t, eng.RunOnce(auth.WithTenant(ctx, tidB), bucket))

    srv := startQueryServer(t, deps)
    defer srv.Close()

    t.Run("sub1_A_sees_own_topology", func(t *testing.T) { /* body contains checkout/redis, not mobile/orders */ })
    t.Run("sub2_B_sees_own_topology", func(t *testing.T) { /* body contains mobile/orders, not checkout/redis */ })
    t.Run("sub3_B_cannot_GET_A_service_by_name", func(t *testing.T) { /* GET /services/checkout with keyB → 404 */ })
    t.Run("sub4_B_list_services_excludes_A", func(t *testing.T) { /* response items have no A-service entries */ })
    t.Run("sub5_missing_Bearer_all_3_routes_401", func(t *testing.T) { /* 3 endpoints × empty key → 401 */ })
    t.Run("sub6_garbage_Bearer_all_3_routes_401", func(t *testing.T) { /* 3 endpoints × bad key → 401 */ })
}
```

**SLICE-2 T11 lesson preserved** in three concrete ways:
- `seedSpansForTenant` runs **outside** any `t.Run` block — every sub-test sees real data.
- `eng.RunOnce` runs **outside** any `t.Run` block — sub2/sub3/sub4 deny-path tests run against actually-populated aggregation tables, not vacuously empty ones.
- Each sub-test only **reads** — no sub-test creates data the next depends on.

Two additional integration tests (separate files for clarity):

- `TestTopoEngine_RunOnce_WritesEdgesAndStats` (engine_test.go) — direct-insert seed, assert 3 edges + N service_stats rows after one bucket.
- `TestTopoEngine_Idempotency_DoubleRun` (engine_test.go) — same bucket run twice, `FINAL` view equals run-once.
- `TestTopoEngine_Catchup_FromLastCompleted` (catchup_test.go) — seed 3 buckets, Catchup processes all 3; second Catchup skips them.
- `TestTopoEngine_CannotWriteEdgeAcrossTenant` (write_isolation_test.go) — attempt to run tenant A's RunOnce with tenant B's ctx; assert no cross-tenant rows leak (defense-in-depth: Layer 1 SQL filter blocks; Layer 2 Row Policy would block; both should fire).

Frontend has three new Playwright specs and several vitest specs (see §9 CI).

## §9 CI matrix

| job | added |
|---|---|
| `backend-unit` | `internal/topoengine/engine_test.go` (bucket math, COALESCE), `internal/query/topology_handler_test.go`, `internal/query/services_handler_test.go`, `internal/chquery/admin_test.go` |
| `backend-integration` (`-tags=integration -timeout=240s`) | `internal/topoengine/cross_tenant_test.go` (6 sub-assertions), `engine_test.go` (RunOnce + Idempotency), `catchup_test.go`, `write_isolation_test.go`, `internal/query/topology_repo_test.go`, `services_repo_test.go` |
| `frontend-unit` (vitest) | `components/ServiceGraph/{ServiceGraph,useForceSimulation}.test.ts`, `views/Overview/{OverviewPage,ServiceCard}.test.ts`, `views/Services/{ServiceDetail,SignalsTab,DependenciesTab}.test.ts`, `views/Topology/TopologyPage.test.ts`, `views/Traces/ServiceMapPanel.test.ts`, `composables/{useTimeWindow,useTopologyApi,useServicesApi}.test.ts` |
| `e2e` (Playwright) | `make seed-topology` runs before; new `tests/e2e/{topology,overview,service-detail}.spec.ts` |
| `lint-ch` | rule extended: `chquery.AdminConn` may only be constructed under `internal/topoengine/` |

## §10 Out of scope / known limitations

**Out of scope (SLICE-3.5 / SLICE-4 / SLICE-5):**

- **⌘K command palette + services injection** — skeleton ADR-0001 §4.4 promised in Slice 0 was YAGNI'd during execution. Building it as part of SLICE-3 would add ~3 tasks. Recommend doing the skeleton + injection together in SLICE-3.5 or SLICE-5.
- **Service detail tabs 3-6** (Runtime / Exceptions / Settings / Alerts) — depend on metrics (SLICE-4) and tenant settings UI (SLICE-5). SLICE-3 ships them as `ComingSoonTab` placeholders.
- **True windowed quantile** (vs `maxIf(p95)` per-bucket-max approximation) — requires `quantileTDigestState` columns in `topo-engine` writes; folded into SLICE-4 metrics aggregation rework.
- **Separate `/services` table view** — SideBar `services` shares `/overview` cards. Table view (sortable by error_rate, p95, etc.) deferred to SLICE-3.5.
- **`/api/v1/services/{name}/traces` + `/logs` quick-link endpoints** — not needed; `SignalsTab` links directly to `/traces?service=X&window=…` (filter already supported by SLICE-1/2).
- **topo-engine HA / leader election** — single instance suffices for MVP; `ReplacingMergeTree` tolerates restart re-runs.
- **`OPTIMIZE TABLE … FINAL` scheduled job** — does not run (too costly); `FROM … FINAL` in queries guarantees correctness.
- **Topology node hover tooltip with inbound/outbound split** — v1 hover shows current node's RED summary only; click navigates to `/services/:name` for breakdown.
- **Topology time-animation / sliding-window replay** — fixed-bucket `TimeWindowPicker` satisfies ADR-0001 §AC "15m/1h/6h/24h replay"; continuous animation deferred.
- **External node reverse navigation** — external nodes don't map to specific traces; UX deferred.
- **Per-trace Service Map external derivation** — peer-attribute inference is unstable within a single trace; external nodes appear only in global topology.

**Known limitations (carried into evidence):**

- **`maxIf(p95)` approximates "worst minute's p95 in the window"**, not the true windowed quantile. Acceptable at MVP; documented in `/api/v1/services` response shape.
- **OTel SDK attribute naming variants** (`peer.service` vs `net.peer.name` vs `server.address`) may cause external node duplication. `COALESCE` order locks priority; alias normalization deferred to SLICE-3.5.
- **`Catchup` capped at 1h gap** — if `topo-engine` has been offline > 1h, older buckets are not back-filled.
- **`top_services` truncation** in `/api/v1/topology` may hide low-volume but interesting edges. Mitigation: external nodes are not truncated; long-tail services accessible via `/services/:name` direct navigation.
- **Service Map subtab in trace detail** uses `Vue computed` on in-memory spans; very large traces (>10k spans) may block the main thread. Web Worker offload deferred.

## §11 References

- Spec: this file (`docs/specs/2026-05-27-slice-3-services-topology-design.md`)
- Predecessors: `docs/specs/2026-05-25-slice-1-trace-design.md` (SLICE-1 closed 2026-05-26), `docs/specs/2026-05-26-slice-2-log-design.md` (SLICE-2 closed 2026-05-26)
- ADRs: `0001-initial-architecture.md` (slice roadmap, topology in Slice 3, force-directed graph), `0002-clickhouse-schema-migrations.md` (forward-only `_schema_migrations`), `0003-query-api-deployment-shape.md` (multi-binary single-image — extended to 4 binaries with topo-engine)
- Lessons learned: `docs/lessons-learned-2026-05-24.md`; PRE-3 lesson on `clickhouse.CustomSetting{}` wrapper required for `custom_*` session settings; SLICE-2 lesson on `suppressNextWatch` URL-watch loop and hoisting cross-tenant test setup outside `t.Run`
- Progress: `docs/claude-progress.json` (SLICE-3 enters `current_focus` on start; `open_questions` records OQ-1..OQ-4)
- Project rules: `CLAUDE.md` "多租户" + "二进制 + 路由划分" + "CH 迁移" blocks (port table updates to include 8084)
- Features ledger: `features.json` SLICE-3 entry (created alongside this spec, `status: proposed`, back-reference to this file)
