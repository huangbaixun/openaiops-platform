---
date: 2026-05-25
topic: slice-1-trace-design
type: feature
status: proposed
features: [SLICE-1]
---

# SLICE-1 design — Trace ingest end-to-end

## Context

Slice 0 shipped a multi-tenant gateway shell + docker-compose stack with no real telemetry path. The three SLICE-1 prerequisites (PRE-1 ch-migrate, PRE-2 split `cmd/query`, PRE-3 `chquery` package + Row Policy + lint) all resolved 2026-05-24, so SLICE-1 has a green field to land the first vertical slice: SDK → OTLP → tenant-scoped CH `traces_v1` → query API → Vue `/traces` page → demo-able.

Brainstormed 2026-05-25 (this session). All four open design choices locked:

1. **Tenant trust** = Bearer-on-receiver. SDK self-declared `tenant.id` is not trusted.
2. **Auth boundary** = ingester (Collector exits the data path). ingester embeds the OTel `otlpreceiver` library and does Bearer→PG→tenant_id→CH+metering in one process.
3. **chquery batch** = extend `chquery.Conn.PrepareBatch` with a `*chquery.Batch` wrapper (closes open task SLICE-1-PREP-1).
4. **Schema scope** = AC #2 columns + a separate `resource_attributes Map(LowCardinality(String), String)`; events/links deferred.
5. **Data source** = `telemetrygen` for CI (deterministic), `hot-r.o.d.` for manual demo (lifelike).
6. **UI scope** = list + waterfall + JSON. Service Map subtab is a `"Coming in SLICE-3"` placeholder; the real force-directed graph component is co-designed with SLICE-3's global topology.

These choices reshaped AC #1, #3, #7, and #9. The evolved text is captured in **§11 AC evolution lock-in** and must be mirrored back into `features.json` SLICE-1 entry before implementation starts.

## §1 Topology + three binaries

```
                                    ┌─────────────────────────┐
                                    │   docker-compose stack  │
SDK / telemetrygen / hot-r.o.d.     │                         │
     │                              │  ┌──────────┐           │
     │  OTLP/gRPC :4317             │  │ Postgres │◄──┐       │
     │  OTLP/HTTP :4318             │  └──────────┘   │       │
     │  Authorization: Bearer <key> │       ▲         │       │
     │                              │       │ pgx     │ pgx   │
     ▼                              │       │         │       │
┌──────────────┐                    │  ┌────┴────┐  ┌─┴──────┐│
│   ingester   │ ─── batch insert ──┼──►   CH    │  │gateway ││
│ (cmd/ingester):                   │  │traces_v1│  │ :8080  │
│  - OTLP rcvr  │ ─── metering ─────┼──►(Postgres)│ │        │
│  - PGResolver │                   │  └─────────┘  └────────┘
│  - chquery    │                       ▲                    │
│  - :4317/4318 │                       │ chquery (read)     │
└──────────────┘                  ┌─────┴─────┐              │
                                  │   query   │              │
                                  │ cmd/query │              │
                                  │   :8081   │              │
                                  └───────────┘              │
                                                             │
                Browser ───► Caddy :443 ──► (per-prefix routing)
                                              ├─ /api/v1/traces*  → query :8081     [added by SLICE-1]
                                              ├─ /api/v1/*        → gateway :8080
                                              └─ /                → frontend :80
```

> SLICE-1 only adds the `/api/v1/traces*` handle. Future slices add their own query-prefix handles (`/api/v1/logs*` in SLICE-2, `/api/v1/services*` + `/api/v1/topology*` in SLICE-3) per the CLAUDE.md "二进制 + 路由划分" rule — no proactive 501 stubs in SLICE-1 (YAGNI; URL shapes locked when actually built).

**Three binaries** (single shared image, multi-target — established by ADR-0003):

| binary | port | role | deps |
|---|---|---|---|
| `cmd/gateway` (existing, unchanged) | 8080 | writes face + admin: `/api/v1/admin*`, `/api/v1/metering*`, `/healthz`, `/livez` | PG |
| `cmd/query` (new) | 8081 | CH read path: `/api/v1/traces*` (SLICE-1), future `/api/v1/logs*` etc. | PG (auth) + CH (data) |
| `cmd/ingester` (new) | 4317 (gRPC), 4318 (HTTP), 8082 (admin) | OTLP receiver + Bearer auth + CH write + metering | PG (auth + metering) + CH (data) |

**Collector is removed from docker-compose data path.** `deploy/otel-collector-config.yaml` is deleted. hot-r.o.d. demo points its OTel exporter at `ingester:4317`, not at a Collector.

**Caddy** routing rule (`deploy/Caddyfile`): only `/api/v1/traces*` added in SLICE-1. Future slices (logs, services, topology, alerts) add their own one-line `handle` block per CLAUDE.md "二进制 + 路由划分" rule. No proactive 501 stubs (YAGNI).

## §2 Tenant propagation (the trust pipeline)

**SDK side.** SDK configures the OTel exporter with:
```
OTEL_EXPORTER_OTLP_ENDPOINT=https://localhost/<grpc-or-http>
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer <api-key>
```
For gRPC this becomes per-RPC metadata. For HTTP this becomes an HTTP header. Both are surfaced inside the ingester via OTel's `client.Info` abstraction.

**Ingester `ConsumeTraces` pipeline** (one OTLP Export call = one iteration):

```
OTLP batch arrives (ResourceSpans[])
  │
  ▼  extract Authorization metadata
  ├─ missing/empty ─► 401 + ingester_auth_missing_total++
  ▼
  strip "Bearer " prefix → api_key
  │
  ▼  auth.PGResolver.Resolve(ctx, api_key) → tenant_id
  ├─ not found / revoked ─► 401 + ingester_auth_invalid_total++
  ▼
  ctx = auth.WithTenant(parent_ctx, tenant_id)
  │
  ▼  flatten ResourceSpans → []SpanRow
  │   - tenant_id           ← server-stamped (NEVER from SDK)
  │   - SDK self-declared `tenant.id` resource attribute is DROPPED
  │   - resource_attributes ← all other resource attrs as Map
  │   - attributes          ← span attrs + `scope.{name,version}` prefixed
  │
  ▼  chquery.PrepareBatch(ctx, "INSERT INTO traces_v1 (tenant_id, ...) VALUES")
  │  batch.Append(row) × N
  │  batch.Send() → CH (Row Policy double-checks tenant_id = getSetting('custom_tenant_id'))
  │
  ├─ ch_prepare_failed / ch_append_failed / ch_send_failed
  │     ─► 500 + ingester_spans_rejected_total{reason}+=N
  ▼
  go metering.Record(WithoutCancel(ctx), tenant_id, "trace", N)
  │   ─ best-effort; failure: log + ingester_metering_failed_total++ (CH already committed)
  │
  ▼
  return OK
```

**Counters emitted by ingester:**

- `ingester_auth_missing_total`
- `ingester_auth_invalid_total`
- `ingester_spans_accepted_total{tenant_id, service}`
- `ingester_spans_rejected_total{reason}` — `reason ∈ {auth_missing, auth_invalid, ch_prepare_failed, ch_append_failed, ch_send_failed, malformed_otlp}`
- `ingester_metering_failed_total`
- `ingester_batch_duration_seconds` (histogram, label by outcome)

**Why drop SDK-declared `tenant.id`** (rather than keep it in `resource_attributes`): prevents a downstream query that filters on `resource_attributes['tenant.id']` from re-introducing the SDK-claimed value as a soft channel. Authoritative `tenant_id` is the column, full stop.

## §3 CH `traces_v1` schema

Migration file: `backend/ch-migrations/20260525120000_create_traces_v1.sql`.

```sql
CREATE TABLE traces_v1 (
    tenant_id           LowCardinality(String),
    trace_id            String CODEC(ZSTD(1)),
    span_id             String CODEC(ZSTD(1)),
    parent_span_id      String CODEC(ZSTD(1)),                  -- '' for root span
    service             LowCardinality(String),
    operation           LowCardinality(String),
    ts                  DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    duration            UInt64 CODEC(T64, LZ4),                 -- nanoseconds
    status              LowCardinality(String),                 -- 'Unset' | 'Ok' | 'Error'
    span_kind           LowCardinality(String),                 -- 'Internal'|'Server'|'Client'|'Producer'|'Consumer'|'Unspecified'
    resource_attributes Map(LowCardinality(String), String),
    attributes          Map(LowCardinality(String), String),

    INDEX idx_trace_id trace_id TYPE bloom_filter(0.01) GRANULARITY 4
) ENGINE = MergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts))
ORDER BY (tenant_id, service, ts)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY tenant_isolation_traces_v1 ON traces_v1
    USING tenant_id = getSetting('custom_tenant_id')
    TO openaiops;
```

**Type choices:**

- `trace_id` / `span_id` / `parent_span_id` are `String` (not `FixedString(32)`/`FixedString(16)`): clickhouse-go v2's batch.Append is more lenient on String, and ZSTD compresses hex strings well. Same choice as SignOz.
- `LowCardinality(String)` for service/operation/status/span_kind: easier to evolve than Enum8 (new values without ALTER TABLE), perf parity at <1k cardinality.
- Map keys also `LowCardinality(String)`: attribute keys repeat heavily within a batch; LC dict compresses key bytes to ~2 bytes per occurrence.

**Codec choices:**

- `ts` uses `Delta + ZSTD(1)`: OTLP batch timestamps are near-monotonic; Delta is optimal.
- `duration` uses `T64 + LZ4`: integer values clustered in a few orders of magnitude; T64 is purpose-built.

**Indexes:**

- `ORDER BY (tenant_id, service, ts)` per AC #2 — satisfies primary query path (within-tenant time window on a service).
- `INDEX idx_trace_id … bloom_filter(0.01) GRANULARITY 4` — supports detail waterfall point-lookup. Granularity 4 = roughly 32k rows per skip block, well-tuned for high-cardinality trace_id.
- No `(service, operation)` skip index in SLICE-1: operation is already in ORDER BY (position 3 after granule pruning); add only if production query patterns prove it needed.

**`PARTITION BY (tenant_id, toYYYYMMDD(ts))` per AC #2 — known scale ceiling.** At ~100 tenants × 90 days = 9000 active partitions, CH's 1000-partition soft limit triggers MERGE backpressure. SLICE-1 MVP is nowhere near this volume; spec records this so a future migration can drop `tenant_id` from PARTITION BY (tenant isolation will still hold via ORDER BY + Row Policy).

Row Policy template comes verbatim from `backend/ch-migrations/README.md` (added during PRE-3 Task 4) — no changes needed.

## §4 `chquery.Batch` wrapper

New file: `backend/internal/chquery/batch.go`. Closes open task SLICE-1-PREP-1.

```go
// Batch wraps clickhouse-go/v2 driver.Batch and enforces tenant scoping
// at construction time + via row-level Row Policy at send time.
type Batch struct {
    b   driver.Batch
    tid string // captured from ctx at PrepareBatch; immutable for batch lifetime
}

// PrepareBatch validates that:
//   - ctx carries a tenant_id (panics otherwise — programmer error)
//   - query is "INSERT INTO <table> (tenant_id, ..." shape (returns error)
// It injects custom_tenant_id setting on the ctx so CH Row Policy fires on Send().
func (cn *Conn) PrepareBatch(ctx context.Context, query string) (*Batch, error) {
    tid, err := auth.TenantID(ctx)
    if err != nil {
        panic(fmt.Errorf("chquery: ctx has no tenant_id (auth middleware did not run?): %w", err))
    }
    if !insertShape.MatchString(query) {
        return nil, fmt.Errorf("chquery: PrepareBatch query must have '(tenant_id,' as first column: %q", query)
    }
    ctxWithSettings := clickhouse.Context(ctx, clickhouse.WithSettings(tenantSettings(tid)))
    b, err := cn.c.PrepareBatch(ctxWithSettings, query)
    if err != nil {
        return nil, fmt.Errorf("chquery: prepare batch: %w", err)
    }
    return &Batch{b: b, tid: tid.String()}, nil
}

// Append checks args[0] is the same tenant_id captured at PrepareBatch.
// Layer 1 (compile/runtime); Row Policy is layer 2 (CH server).
func (b *Batch) Append(args ...any) error {
    if len(args) == 0 {
        return errors.New("chquery: batch Append needs at least tenant_id as first arg")
    }
    s, ok := args[0].(string)
    if !ok || s != b.tid {
        return fmt.Errorf("chquery: batch Append first arg must be tenant_id %q, got %T %v", b.tid, args[0], args[0])
    }
    return b.b.Append(args...)
}

func (b *Batch) AppendStruct(v any) error { return b.b.AppendStruct(v) }  // Row Policy is the backstop
func (b *Batch) Send() error              { return b.b.Send() }
func (b *Batch) Abort() error             { return b.b.Abort() }
func (b *Batch) IsSent() bool             { return b.b.IsSent() }
func (b *Batch) Rows() int                { return b.b.Rows() }
```

**Intentionally NOT exposed:**

- `Column(int) driver.BatchColumn` — columnar append API. Higher throughput but ingester MVP doesn't need it; add when performance data demands.
- `AsyncInsert` — defer to post-SLICE-1 once we have throughput data.
- Underlying `driver.Batch` accessor — the lint rule (`make lint-ch`) keeps callers inside the chquery boundary.

**Tests:**

- `backend/internal/chquery/batch_test.go` (unit) — ctx without tenant_id panics; non-INSERT-shape query errors; Append with wrong tenant_id errors.
- Extend `backend/internal/chquery/conn_smoke_test.go` (dockertest CH `-tags=integration`) — full PrepareBatch → Append × N → Send → SELECT round-trip + ctx-switch cross-tenant negative.

## §5 Query API (`cmd/query` :8081)

**Code layout:**

```
backend/
  cmd/query/main.go             # config → PG → CH → resolver → router → server (mirrors cmd/gateway)
  internal/query/
    router.go                   # chi router; auth.Middleware(resolver); registers traces routes
    traces_handler.go           # HTTP handler: parse params → call repo → JSON encode
    traces_repo.go              # SQL templates; uses chquery.Conn.Query
    traces_repo_test.go         # dockertest CH; cross-tenant mini-test
    types.go                    # request/response shapes
```

### `GET /api/v1/traces` (list)

**Query params:**

| param | type | default | required | notes |
|---|---|---|---|---|
| `ts_from` | RFC3339 nano | `now - 1h` | optional | time window start |
| `ts_to` | RFC3339 nano | `now` | optional | time window end |
| `service` | string | `""` | optional | any-span match (Jaeger UI semantics) |
| `operation` | string | `""` | optional | any-span match |
| `min_duration_ms` | float | `0` | optional | any-span `duration ≥ threshold` |
| `limit` | int | `100`, max `1000` | optional | |
| `offset` | int | `0` | optional | known limitation: offset paging drifts on streaming data — cursor paging in a post-SLICE-1 enhancement |
| `sort` | enum `ts \| duration` | `ts` | optional | handler whitelist; protects template substitution |
| `order` | enum `asc \| desc` | `desc` | optional | same |

**Response shape:**

```json
{
  "items": [
    {
      "trace_id": "0af7651916cd43dd8448eb211c80319c",
      "root_service": "frontend",
      "root_operation": "HTTP GET /dispatch",
      "start_ts": "2026-05-25T12:00:00.123456789Z",
      "duration_ns": 234560000,
      "span_count": 12,
      "services": ["frontend", "customer", "driver"]
    }
  ],
  "has_more": true
}
```

`has_more` is computed via the `limit + 1` trick — query for `limit + 1` rows, set `has_more = true` if returned. No `total` (count-distinct over GROUP BY is expensive at MVP volume targets; we'd revisit only if UI demands page numbers).

**SQL template:**

```sql
SELECT
    trace_id,
    argMin(service,   ts) AS root_service,
    argMin(operation, ts) AS root_operation,
    min(ts)               AS start_ts,
    sum(duration)         AS approx_duration_ns,
    count()               AS span_count,
    arraySlice(groupUniqArray(service), 1, 10) AS services
FROM traces_v1
WHERE tenant_id = ?
  AND ts >= ? AND ts < ?
  AND (? = '' OR service   = ?)
  AND (? = '' OR operation = ?)
  AND duration >= ?
GROUP BY trace_id
ORDER BY {{.SortCol}} {{.Order}}     -- Go-template substitution; handler whitelist enforces ts|duration + asc|desc
LIMIT ? OFFSET ?
```

`tenant_id = ?` satisfies `MustTenantScope`. Handler does not pass tid explicitly; `chquery` injects from ctx.

### `GET /api/v1/traces/{trace_id}` (waterfall)

**Response:**

```json
{
  "trace_id": "...",
  "spans": [
    {
      "span_id": "...",
      "parent_span_id": "",                       // empty = root
      "service": "frontend",
      "operation": "HTTP GET /dispatch",
      "ts": "...",
      "duration_ns": 234560000,
      "status": "Ok",
      "span_kind": "Server",
      "resource_attributes": {"host.name": "fe-01", "deployment.environment": "prod"},
      "attributes": {"http.method": "GET", "http.status_code": "200"}
    }
  ]
}
```

Flat span list; frontend builds the parent→children tree to render the waterfall (avoids server-side recursion + nested JSON).

`404` if no rows match (`trace_id` not present under this tenant). This is the case cross-tenant E2E exercises (§9).

**SQL:**

```sql
SELECT span_id, parent_span_id, service, operation,
       ts, duration, status, span_kind,
       resource_attributes, attributes
FROM traces_v1
WHERE tenant_id = ?
  AND trace_id  = ?
ORDER BY ts ASC
```

`trace_id =` hits the bloom_filter `idx_trace_id` so CH skips non-matching granules.

### Caddy routing update

```caddyfile
localhost {
    tls internal

    # query 前缀优先匹配
    handle /api/v1/traces* {
        uri strip_prefix /api
        reverse_proxy query:8081
    }

    # 其它 /api/* 落 gateway
    handle /api/* {
        uri strip_prefix /api
        reverse_proxy gateway:8080
    }

    handle { reverse_proxy frontend:80 }
}
```

### HTTP errors

- `400` — invalid ts, sort/order outside whitelist, limit out of bounds
- `401` — handled by `auth.Middleware` (missing/invalid Bearer)
- `404` — trace_id not present under this tenant
- `500` — CH/PG error (slog.Error + counter)

## §6 Ingester service (`cmd/ingester`)

**Code layout:**

```
backend/
  cmd/ingester/main.go              # config → PG → CH → resolver → receiver → run
  internal/ingest/
    receiver.go                     # OTel otlpreceiver factory; wires custom consumer
    auth.go                         # gRPC client.Metadata / HTTP header → Bearer
    consume.go                      # consumer.Traces impl: auth → spans → CH → metering
    spans.go                        # ptrace.Traces → []SpanRow
    metering.go                     # async PG metering writer
    metrics.go                      # Prometheus counters/histograms
    admin.go                        # /healthz /livez /metrics on :8082
    types.go                        # SpanRow
    *_test.go
```

**Why OTel `otlpreceiver` library, not hand-rolled gRPC+HTTP:** the library handles gzip, content-type negotiation (proto/JSON over HTTP), payload-size limits, connection lifecycle, retry semantics — all OTLP spec compliance for free. Cost: ~80 LOC of `component.Component` lifecycle boilerplate (Start/Shutdown) and the `consumer.Traces` interface signature locks us to the collector's pdata types. Acceptable.

**Receiver config (built inline, not YAML):**

```yaml
protocols:
  grpc:
    endpoint: 0.0.0.0:4317
  http:
    endpoint: 0.0.0.0:4318
```

**`ConsumeTraces` pseudocode:**

```go
func (c *consumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
    bearer, err := extractBearer(ctx)
    if err != nil {
        c.metrics.AuthMissing.Inc()
        return statusUnauthenticated("missing bearer")
    }
    tid, err := c.resolver.Resolve(ctx, bearer)
    if err != nil {
        c.metrics.AuthInvalid.Inc()
        return statusUnauthenticated("invalid bearer")
    }
    ctx = auth.WithTenant(ctx, tid)

    rows := spansToRows(td)             // drops SDK self-declared tenant.id resource attr
    if len(rows) == 0 {
        return nil
    }

    batch, err := c.ch.PrepareBatch(ctx, insertTracesV1Stmt)
    if err != nil {
        c.metrics.SpansRejected.WithLabelValues("ch_prepare_failed").Add(float64(len(rows)))
        return statusInternal(err)
    }
    for _, r := range rows {
        if err := batch.Append(tid.String(), r.TraceID, /* ... */); err != nil {
            _ = batch.Abort()
            c.metrics.SpansRejected.WithLabelValues("ch_append_failed").Add(float64(len(rows)))
            return statusInternal(err)
        }
    }
    if err := batch.Send(); err != nil {
        c.metrics.SpansRejected.WithLabelValues("ch_send_failed").Add(float64(len(rows)))
        return statusInternal(err)
    }
    c.metrics.SpansAccepted.WithLabelValues(tid.String()).Add(float64(len(rows)))

    // best-effort metering — CH commit is the real commit
    go c.metering.Record(context.WithoutCancel(ctx), tid, "trace", len(rows))

    return nil
}
```

**Metering best-effort policy:** CH commit returns OK to the SDK. The metering write to PG runs in a detached goroutine; failure increments `ingester_metering_failed_total` and logs an error but does not surface to the SDK. SLICE-2+ may add a retry queue / failed-metering ledger.

**Env vars:**

| var | default | notes |
|---|---|---|
| `INGESTER_OTLP_GRPC_ADDR` | `0.0.0.0:4317` | container-side bind |
| `INGESTER_OTLP_HTTP_ADDR` | `0.0.0.0:4318` | |
| `INGESTER_ADMIN_ADDR` | `0.0.0.0:8082` | not exposed to host by default |
| `INGESTER_OTLP_GRPC_HOST_PORT` | `4317` | host-side port mapping; overridable in `deploy/.env.local` for SignOz collision |
| `INGESTER_OTLP_HTTP_HOST_PORT` | `4318` | |
| `DATABASE_URL` | from compose | PG handle for resolver + metering |
| `CLICKHOUSE_DSN` | from compose | CH handle |

**Graceful shutdown:**

- SIGTERM → `receiver.Shutdown(ctx)` (stop accepting, drain in-flight, 30s timeout)
- Wait for pending metering goroutines (additional 5s, `sync.WaitGroup`)
- Close CH → close PG → exit 0

**Dockerfile:** reuses `backend/Dockerfile` multi-target pattern (ADR-0003 single image / multiple binaries). compose `ingester` service runs `/app/ingester` from the same image as `gateway` and `query`.

## §7 Frontend `/traces` page

**Files:**

```
frontend/src/
  views/Traces/
    TracesList.vue              # NDataTable + filter bar (service/operation/min_duration/ts range)
    TraceDetail.vue             # NTabs: Waterfall | JSON | Service Map
    WaterfallChart.vue          # Hand-rolled SVG, ~100 LOC, no D3
  api/traces.ts                 # typed client over existing axios + auth interceptor
  composables/useTraces.ts      # list/detail state wrappers
  i18n/locales/{zh-CN,en-US}/traces.ts
```

**Router additions:**

- `/traces` → `TracesList`
- `/traces/:traceId` → `TraceDetail`

**SideBar:** add `Traces` nav item.

**Waterfall renderer:** pure SVG, no D3 or other graph lib:

- Each span = one `<rect>`. `x = (span.ts − trace.start_ts) / total_duration × width`. `width = duration / total_duration × pixel_width`.
- Children indented by parent_span_id BFS depth. `y = depth × row_height`.
- Color hash `service.name` → HSL hue (same service = same color).
- Hover tooltip: operation + duration + first-N attributes.

Force-directed graph component is deferred to SLICE-3 to share with global topology.

**Service Map subtab:** `<NEmpty description="$t('traces.serviceMapComingSoon')" />`. Locked in §11 AC #7 evolution.

**Playwright spec `frontend/e2e/traces.spec.ts`:**

1. Login → /traces → list renders (with telemetrygen pre-seeded data)
2. Click row → /traces/:id → Waterfall tab shows N `<rect>` elements
3. Switch JSON tab → raw JSON visible
4. Switch Service Map tab → "Coming in SLICE-3" copy visible
5. Re-login as tenant B → /traces → empty list (cross-tenant UX assertion; the real security assertion is in §9)

## §8 Cross-tenant E2E (AC #8)

Lives in **Go integration test, not Playwright.** Security assertions must run against the data layer; the frontend can fake success via cached UI state.

**File:** `backend/internal/ingest/cross_tenant_test.go` (`//go:build integration`).

```go
//go:build integration

func TestSlice1_CrossTenantIsolation(t *testing.T) {
    pgPool, chPool := dockertestUp(t)
    runMigrations(t, pgPool, chPool)

    tidAcme, keyAcme := seedTenant(t, pgPool, "acme")
    tidBeta, keyBeta := seedTenant(t, pgPool, "beta")

    ingester     := startInProcessIngester(t, pgPool, chPool)
    queryHandler := startInProcessQueryHandler(t, pgPool, chPool)

    traceID := sendOTLP(t, ingester.GRPCAddr, keyAcme, fixtureSpans(5))
    waitForCHRows(t, chPool, tidAcme, 5, 10*time.Second)

    t.Run("acme sees own traces",            func(t *testing.T) { assertListCount(t, queryHandler, keyAcme, 1) })
    t.Run("acme sees own span count",        func(t *testing.T) { assertDetailSpans(t, queryHandler, keyAcme, traceID, 5) })
    t.Run("beta sees zero traces",           func(t *testing.T) { assertListCount(t, queryHandler, keyBeta, 0) })
    t.Run("beta gets 404 on acme trace_id",  func(t *testing.T) { assertDetail404(t, queryHandler, keyBeta, traceID) })
    t.Run("no bearer → 401",                 func(t *testing.T) { assertList401(t, queryHandler, "") })
    t.Run("garbage bearer → 401",            func(t *testing.T) { assertList401(t, queryHandler, "deadbeef") })
}
```

Six sub-tests cover isolation + observability of the isolation. OTLP fixture is hand-crafted (5 spans, deterministic IDs) via the in-process OTLP/gRPC client — no need to start telemetrygen as a subprocess.

## §9 CI matrix (AC #10)

**Job updates** in `.github/workflows/`:

| job | change |
|---|---|
| `backend-unit` | picks up new `chquery.Batch` unit tests, `internal/ingest/*` unit tests, `internal/query/*` unit tests via default `go test ./...` |
| `backend-integration` (**new job**) | runs `go test -tags=integration -timeout=240s ./...` — dockertest suite (existing apikey + new chquery batch round-trip + new ingest cross_tenant + new query traces_repo) |
| `frontend-unit` | adds `views/Traces/*.spec.ts` vitest (NDataTable + Waterfall pure-render assertions) |
| `e2e` | seeds traces via `make seed-traces` (in-process telemetrygen against each tenant), then runs Playwright `traces.spec.ts` |
| `lint-ch` | unchanged; existing `make lint-ch` already scans all of `internal/` so the new `ingest/` and `query/` packages are covered automatically |

**New Makefile targets:**

- `make seed-traces` — Go helper that opens an in-process OTLP/gRPC client against the running ingester and posts a deterministic fixture for each seeded tenant. Used by the e2e job; replaces the would-be dependency on hot-r.o.d.
- `make demo-traces` — `docker compose --profile demo up hot-r.o.d.` — visible business traffic for human-eye demos; CI does not run this.

## §10 Data sources

**Two sources, two contexts:**

| context | source | why |
|---|---|---|
| CI / `make seed-traces` | `telemetrygen` (OTel contrib) | deterministic span count, deterministic IDs, settable headers → cross-tenant E2E is reproducible |
| `make demo` / manual review | `hot-r.o.d.` | continuous lifelike business traffic; UI screenshots look real |

`telemetrygen` is invoked **in-process** by the seed-traces helper (we vendor the OTLP/gRPC client, not the CLI binary). `hot-r.o.d.` runs as a docker-compose `--profile demo` service that depends on `ingester` and is excluded from `docker compose up` by default.

## §11 AC evolution lock-in

These four ACs in `features.json` SLICE-1 must be updated to match the design. The lock-in prevents future drift back to a Collector-centric or honor-system tenant trust model.

| AC# | original wording | evolved wording |
|---|---|---|
| #1 | "OTel Collector custom processor (Go) reads tenant.id from OTLP resource attribute or 'X-Tenant-Id' header on HTTP receiver; rejects spans missing tenant.id with 400/dropped + metric" | "**ingester OTLP receiver** reads `Authorization: Bearer <api-key>` from gRPC metadata / HTTP header; resolves via `auth.PGResolver` to obtain authoritative `tenant_id`; rejects missing or invalid Bearer with **401** + `ingester_auth_{missing,invalid}_total` counter. SDK-declared `tenant.id` resource attribute is **dropped** (cannot be spoofed)." |
| #3 | "Go ingester service: receives OTLP from Collector, writes batches to CH traces_v1, propagates tenant_id from processor" | "Go ingester service is **itself the OTLP receiver** (gRPC :4317 + HTTP :4318, embeds `go.opentelemetry.io/collector/receiver/otlpreceiver`); writes batches to CH traces_v1 via `chquery.PrepareBatch`; tenant_id is **server-stamped after PG resolve** (not propagated)." |
| #7 | "Frontend /traces page (Vue): list (sortable by ts/duration) + detail waterfall + JSON / Service Map subtabs (port from openapm/js/pages-traces.js)" | "Frontend /traces page (Vue): list (sortable by ts/duration) + detail waterfall (hand-rolled SVG, no D3) + JSON subtab. **Service Map subtab is a `\"Coming in SLICE-3\"` placeholder**; the real force-directed graph component is co-designed with SLICE-3 global topology." |
| #9 | "Per-ingest metering event written to PG metering_events (tenant_id, signal_type='trace', count=spans_in_batch)" | "Per-OTLP-Export-call metering event written to PG metering_events (tenant_id, signal_type='trace', count=spans_in_batch). Write is **best-effort**: CH commit returns OK to SDK regardless; PG metering failure increments `ingester_metering_failed_total` + logs. A retry queue is post-SLICE-1." |

All other ACs (#2, #4, #5, #6, #8, #10) unchanged in wording; implementation details captured in §3-§9 above.

## §12 Out of scope / known limitations

- **Sampling decisions** — ingest 100%. Tail-based sampling is SLICE-5+.
- **AsyncInsert / column-mode batch** — `chquery.Batch` exposes only row-mode `Append`. Add when throughput data shows the need.
- **`cmd/query` for non-traces endpoints** — `/api/v1/logs*` etc. routes do not get proactive 501 stubs. Future slices add their own Caddy lines per the CLAUDE.md routing rule.
- **Cursor pagination on `/api/v1/traces`** — SLICE-1 ships offset paging with known drift on streaming data. Cursor is a post-SLICE-1 enhancement; ticket added once a concrete user need surfaces.
- **`PARTITION BY (tenant_id, toYYYYMMDD(ts))` scale ceiling** — ~9000 active partitions at 100 tenants × 90 days hits CH's MERGE backpressure. A migration to `PARTITION BY toYYYYMMDD(ts)` only (tenant isolation still held by ORDER BY + Row Policy) is the documented path. Trigger: tenant count > 30 or partition count > 1000.
- **OTLP events / links** — schema does not include them. Waterfall view has no event-timeline markers. Cross-reference to logs (SLICE-2) is the planned mitigation.
- **Collector** — removed from data path. Anyone wanting Collector-style enrichment (e.g., `attributesprocessor` rules) must run their own and point at `ingester:4317`. spec note in CLAUDE.md mentions this.
- **PG resolver hot path** — Slice 0 known_drift D1 (full-scan + bcrypt-verify on every request) now applies to ingester too. SLICE-1 does not fix it. Mitigation: in-memory cache (LRU by key prefix, 5-min TTL) is a SLICE-2 prerequisite if ingester throughput targets exceed ~50 req/s.

## §13 SLICE-1-PREP-1 closure

`docs/claude-progress.json` open_tasks SLICE-1-PREP-1 ("chquery.Conn batch-insert support before SLICE-1 ingester") is resolved by §4 (extend `chquery.Conn.PrepareBatch` with `*chquery.Batch` wrapper). After this spec lands, the task should be moved from `open_tasks` to `resolved_questions` with a back-reference to this spec.

## §14 Implementation phasing (high-level)

A detailed plan goes to `docs/plans/2026-05-25-slice-1-plan.md` via the `harness:writing-plans` skill (next session step). High-level phasing:

1. **CH migration** — `traces_v1` table + Row Policy migration; smoke-tested via `make migrate-ch-up` against fresh CH (1 day).
2. **chquery.Batch** — wrapper + unit tests + dockertest extension (1 day).
3. **cmd/ingester scaffold** — config, PG/CH wiring, `auth.PGResolver` reuse, admin endpoints (1 day).
4. **OTel otlpreceiver embed** — receiver factory, `consumer.Traces` impl skeleton (2 days).
5. **Bearer auth + spans→rows + CH write** — `consume.go` happy path (2 days).
6. **Metering best-effort async** — goroutine + metrics + tests (1 day).
7. **cmd/query scaffold** — mirrors gateway pattern; PG + CH handles; chi router (1 day).
8. **traces_handler + traces_repo (list)** — SQL template, filter parsing, has_more pagination (2 days).
9. **traces_handler + traces_repo (detail)** — bloom_filter lookup, 404 semantics (1 day).
10. **Caddy routing update + docker-compose updates** — ingester service, query service, hot-r.o.d. demo profile, remove Collector (1 day).
11. **Frontend /traces list + filter bar + i18n** (2 days).
12. **Frontend /traces detail + Waterfall SVG + JSON tab + Service Map placeholder** (2 days).
13. **Cross-tenant E2E integration test** (1 day).
14. **CI matrix updates + new `backend-integration` job + new Makefile targets** (1 day).
15. **e2e Playwright `traces.spec.ts`** (1 day).
16. **AC verification pass + docs polish** (1 day).

Estimate: **20-21 ideal engineer days** (matches features.json `estimated_effort_days: 21`).

## References

- Spec: ADR-0001 (initial architecture, §3.3 multi-tenant defense)
- Spec: ADR-0002 (CH migrations, forward-only)
- Spec: ADR-0003 (cmd/query split + chquery boundary)
- Plan: docs/plans/2026-05-24-pre-3-must-tenant-scope-plan.md (chquery package baseline)
- Lessons: docs/lessons-learned-2026-05-24.md
- Open task closed by this spec: `docs/claude-progress.json` SLICE-1-PREP-1
