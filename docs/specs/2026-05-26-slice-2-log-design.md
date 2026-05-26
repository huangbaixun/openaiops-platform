---
date: 2026-05-26
topic: slice-2-log-design
type: feature
status: proposed
features: [SLICE-2]
---

# SLICE-2 design — Log ingest end-to-end

## Context

SLICE-1 (closed 2026-05-26, 10/10 ACs) shipped the trace vertical: `cmd/ingester` → CH `traces_v1` (Row Policy) → `cmd/query` → Vue `/traces` page. SLICE-2 adds the log signal on the same architectural pattern, plus closes drift D4 (Caddy + nginx duplicated routing) that SLICE-1 left as a known structural issue.

Brainstormed 2026-05-26 (this session). Four design choices locked:

1. **Binary topology** = split. New `cmd/log-ingester` binary with its own ports (4327/4328/8083), symmetric to PRE-2's reasoning for `cmd/query`. Independent blast radius from trace ingest; mirrors the multi-binary single-image pattern in ADR-0003.
2. **Log↔trace correlation** = first-class. `trace_id` and `span_id` are top-level CH columns (not in attribute Map) with bloom_filter indexes. Cross-jump UX both ways: log row → `/traces/:trace_id`; trace detail gains a "Logs" subtab; clicking a span in the waterfall narrows to that span's logs (packaging B).
3. **Routing** = consolidate to Caddy-only ingress. `frontend/nginx.conf` `/api/*` blocks are deleted; the SPA-loading container becomes a dumb static asset server. Closes drift D4 root cause; any future query route adds one line to `Caddyfile`, zero to `nginx.conf`.
4. **Search scope** = structured filters + body substring (`positionUTF8`), no text index. service / severity / ts range / trace_id / span_id / body_contains. Full-text index (`tokenbf_v1`) deferred until query patterns are real.

A fifth packaging choice (`B. A + span-granularity + JSON pretty-print`) added: clicking a span in the waterfall scopes the Logs subtab to `?trace_id=X&span_id=Y`; JSON bodies pretty-print on row expand.

## §1 Topology + four binaries

```
                                    ┌─────────────────────────┐
                                    │   docker-compose stack  │
SDK / telemetrygen / hot-r.o.d.     │                         │
     │                              │  ┌──────────┐           │
     │  OTLP/gRPC :4317 (traces)    │  │ Postgres │◄──┐       │
     │  OTLP/HTTP :4318 (traces)    │  └──────────┘   │       │
     │  OTLP/gRPC :4327 (logs)      │       ▲         │       │
     │  OTLP/HTTP :4328 (logs)      │       │ pgx     │ pgx   │
     │  Authorization: Bearer <key> │       │         │       │
     ▼                              │  ┌────┴───────┐ │       │
┌──────────────┐                    │  │     CH     │ │       │
│   ingester   │ ── trace batch ────┼──► traces_v1  │ │       │
│   :4317/4318 │                    │  │            │ │       │
│  (unchanged) │                    │  │  logs_v1   │ │       │
└──────────────┘                    │  └────┬───────┘ │       │
┌──────────────┐                    │       │         │       │
│ log-ingester │ ── log batch ──────┼───────┘         │       │
│   :4327/4328 │                    │                 │       │
│   :8083 admin│                    │  ┌─────────┐  ┌─┴──────┐│
│  (new)       │                    │  │ gateway │  │ query  │
│  - OTLP rcvr │                    │  │  :8080  │  │ :8081  │
│  - PGResolver│                    │  └─────────┘  └────────┘
│  - chquery   │                    │                  ▲      │
│  - logingest │                    │                  │      │
└──────────────┘                    └──────────────────┼──────┘
                                                       │
                Browser ───► Caddy :443 ────────► (per-prefix routing — single ingress)
                                                  ├─ /api/v1/traces*  → query :8081
                                                  ├─ /api/v1/logs*    → query :8081     [added by SLICE-2]
                                                  ├─ /api/v1/*        → gateway :8080
                                                  └─ /                → frontend :80 (static SPA only)
```

> SLICE-2 adds the `/api/v1/logs*` handle and **deletes** the `/api/*` blocks from `frontend/nginx.conf` (drift D4 fix per §7). Frontend container goes back to serving only static SPA assets.

**Four binaries** (single shared image, multi-target; new binary in **bold**):

| binary | port | role | deps |
|---|---|---|---|
| `cmd/gateway` (unchanged) | 8080 | admin / metering / health | PG |
| `cmd/query` (unchanged code; +1 route file) | 8081 | CH read path; gains `/api/v1/logs*` | PG (auth) + CH (data) |
| `cmd/ingester` (unchanged) | 4317 / 4318 / 8082 | OTLP **trace** receiver | PG + CH |
| **`cmd/log-ingester`** (new) | **4327 / 4328 / 8083** | OTLP **log** receiver (otlpreceiver embed, Bearer auth, CH write, metering) | PG + CH |

Port rationale: `+10` offset from trace ports keeps signal pairs visually aligned (4317↔4327, 4318↔4328, 8082↔8083) and avoids clashes with common metrics-port choices (e.g., 4319/4320). Host port env vars: `LOG_INGESTER_OTLP_GRPC_HOST_PORT` / `LOG_INGESTER_OTLP_HTTP_HOST_PORT` added to `deploy/.env.example` (defaults 4327/4328).

**Caddy** routing rule added (`deploy/Caddyfile`) — one new `handle /api/v1/logs* { uri strip_prefix /api; reverse_proxy query:8081 }` block, listed before the catch-all `/api/*` per the existing first-match-wins pattern.

## §2 Tenant propagation (the trust pipeline)

Identical to SLICE-1, with `ConsumeLogs` instead of `ConsumeTraces`. The Bearer-on-receiver pattern stays load-bearing; SDK self-declared `tenant.id` is dropped.

**SDK side.** SDK configures the OTel log exporter with:
```
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=https://localhost/v1/logs   # HTTP
# or for gRPC:
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=https://localhost:4327
OTEL_EXPORTER_OTLP_LOGS_HEADERS=Authorization=Bearer <api-key>
```

**Log ingester `ConsumeLogs` pipeline** (one OTLP Export call = one iteration):

```
OTLP batch arrives (ResourceLogs[])
  │
  ▼  extract Authorization metadata (gRPC) or header (HTTP)
  ├─ missing/empty ─► 401 + log_ingester_auth_missing_total++
  ▼
  strip "Bearer " prefix → api_key
  │
  ▼  auth.PGResolver.ResolveBearer(ctx, api_key) → tenant
  ├─ not found / revoked ─► 401 + log_ingester_auth_invalid_total++
  ▼
  ctx = auth.WithTenant(parent_ctx, tenant.ID, tenant.Name)
  │
  ▼  flatten ResourceLogs → []LogRow
  │   - tenant_id           ← server-stamped (NEVER from SDK)
  │   - SDK self-declared `tenant.id` resource attribute is DROPPED
  │   - body                ← bodyAsString(record.Body()) — JSON-encode Map/Slice; AsString() otherwise
  │   - trace_id, span_id   ← hex-encode; empty (all-zero) becomes ""
  │   - resource_attributes ← all other resource attrs as Map
  │   - attributes          ← record attrs as Map
  │
  ▼  chquery.PrepareBatch (pins tenant_id; first Append arg validated)
  │
  ▼  Append each row; Send; metering.Enqueue(signal_type='log', count=len(rows))
```

**`IncludeMetadata: true` on both gRPC and HTTP transports from day 1.** SLICE-1 T10 + T13 caught this bug for traces; we do not replay it for logs.

## §3 CH `logs_v1` schema

Per ADR-0001 §3.3 (tenant-first ordering) and ADR-0002 (forward-only migrations). Migration file: `backend/ch-migrations/20260527120000_create_logs_v1.sql`.

```sql
CREATE TABLE IF NOT EXISTS logs_v1 (
    tenant_id           LowCardinality(String),
    ts                  DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    observed_ts         DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    service             LowCardinality(String),
    severity_text       LowCardinality(String),
    severity_number     UInt8 CODEC(T64, ZSTD(1)),
    body                String CODEC(ZSTD(3)),
    trace_id            String CODEC(ZSTD(1)),
    span_id             String CODEC(ZSTD(1)),
    trace_flags         UInt8 CODEC(T64, ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String),
    attributes          Map(LowCardinality(String), String),
    INDEX idx_trace_id  trace_id        TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_span_id   span_id         TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX idx_severity  severity_number TYPE minmax GRANULARITY 4
) ENGINE = MergeTree
-- Partition ceiling ~9000 active partitions at 100 tenants × 90 days.
-- Logs typically 10× trace volume; revisit toYYYYMM if MergeTree part count pressure surfaces.
PARTITION BY (tenant_id, toYYYYMMDD(ts))
ORDER BY (tenant_id, service, ts)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_logs_v1 ON logs_v1
    USING tenant_id = getSetting('custom_tenant_id') TO openaiops;
```

**Design notes:**

- `body String CODEC(ZSTD(3))` — higher compression than traces (`ZSTD(1)`) because log payloads repeat heavily.
- OTLP `body` is `AnyValue` (string / int / bool / kvlist / array). Non-string bodies are JSON-encoded on ingest via `bodyAsString()` (§6). The frontend pretty-prints when `JSON.parse(body)` succeeds.
- `observed_ts` is kept separately from `ts` — OTLP distinguishes SDK timestamp vs collector observed time; useful for clock-skew debugging in the AI sibling later. Storage cost is small.
- `trace_id` + `span_id` are top-level columns with `bloom_filter(0.01)` indexes — supports both cross-jump scopes (per-trace and per-span) without full scans. Empty (all-zero OTLP) is stored as `""`; the `?trace_id=` / `?span_id=` filter exact-matches.
- `severity_number` `minmax` index supports `severity_number >= 17` (ERROR+) range scans efficiently. `severity_text` filter goes via the `LowCardinality` dictionary.
- `body` substring search uses `positionUTF8(body, ?) > 0` — full scan within partition-pruned range. Acceptable at MVP; documented as known limitation (§10).
- `PARTITION BY (tenant_id, toYYYYMMDD(ts))` mirrors `traces_v1`. The logs-vs-traces volume asymmetry (typically 10×) puts more pressure on the partition count ceiling; documented as a SLICE-3 admin concern.
- **Row Policy** `tenant_isolation_logs_v1` is layer-2 defense identical to `traces_v1`. Powered by the `custom_tenant_id` session setting that `chquery.Conn` injects.

## §4 Query API (`cmd/query` :8081)

**Tenant scoping (load-bearing — read first).** `tenant_id` is **never** a client-supplied query parameter. Every request to `/api/v1/logs` carries `Authorization: Bearer <api-key>`; `auth.Middleware(resolver)` on `cmd/query` (`internal/query/router.go:32`) resolves it to the authoritative `tenant_id` and injects it into `ctx`. The handler reads only the user-facing filters (`service`, `severity`, …). The repo calls `chquery.Conn.Query(ctx, sql, args...)` whose `MustTenantScope` (1) panics if ctx has no tenant, (2) panics if the SQL lacks `WHERE tenant_id = ?` as the first predicate, (3) prepends the tenant_id as `args[0]`, and (4) sets the CH session `custom_tenant_id` so the Row Policy `tenant_isolation_logs_v1` fires as layer-2 defense. `make lint-ch` blocks bare `clickhouse-go` imports outside `internal/chquery` as layer 3. Symmetric to SLICE-1 AC #5 + AC #6; verified by §8 cross-tenant E2E for logs.

### `GET /api/v1/logs` (list)

| query param | type | semantics |
|---|---|---|
| `service` | repeated string | `service IN (?, …)` — e.g., `?service=cartservice&service=checkout` |
| `severity` | repeated string | `severity_text IN (?, …)` — e.g., `?severity=ERROR&severity=FATAL` |
| `ts_from` | RFC3339 | `ts >= ?`; default `now - 1h` |
| `ts_to` | RFC3339 | `ts < ?`; default `now + 1s` (drift D1 mitigation — see §10) |
| `trace_id` | hex (32) | `trace_id = ?` — drives the cross-jump from trace detail |
| `span_id` | hex (16) | `span_id = ?` — drives per-span scoping (packaging B) |
| `body_contains` | string | `positionUTF8(body, ?) > 0` — full scan within partition selection |
| `limit` | int | default 50, max 500 |
| `offset` | int | default 0; response includes `has_more` via limit+1 trick |

Sort: `ts DESC` only (time-series; no other order makes sense in v1).

**Response (`200 OK`):**

```json
{
  "items": [
    {
      "ts": "2026-05-27T10:23:44.123456789Z",
      "observed_ts": "2026-05-27T10:23:44.124000000Z",
      "service": "cartservice",
      "severity_text": "ERROR",
      "severity_number": 17,
      "body": "checkout failed: ...",
      "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
      "span_id": "00f067aa0ba902b7",
      "trace_flags": 1,
      "resource_attributes": {"service.namespace": "demo"},
      "attributes": {"http.status_code": "500"}
    }
  ],
  "has_more": true
}
```

**Errors:**

| status | when |
|---|---|
| 400 | bad RFC3339, bad hex for `trace_id`/`span_id`, `limit` out of range |
| 401 | missing/invalid Bearer (gateway middleware) |
| 500 | CH error (logged with slog.Error; response body is generic) |

**Implementation pointers:**
- Route handler: `backend/internal/query/logs_handler.go` (new file, mirrors `traces_handler.go`)
- Repo: `backend/internal/query/logs_repo.go` (mirrors `traces_repo.go`)
- All SQL via `chquery.MustTenantScope`; SQL skeleton:

```sql
SELECT ts, observed_ts, service, severity_text, severity_number, body,
       trace_id, span_id, trace_flags, resource_attributes, attributes
FROM logs_v1
WHERE tenant_id = ?                       -- AUTO-PREPENDED from ctx by chquery, NEVER from URL
  AND ts >= ? AND ts < ?
  AND (length(?) = 0 OR has(?, service))
  AND (length(?) = 0 OR has(?, severity_text))
  AND (? = '' OR trace_id = ?)
  AND (? = '' OR span_id  = ?)
  AND (? = '' OR positionUTF8(body, ?) > 0)
ORDER BY ts DESC
LIMIT ? OFFSET ?
```

No detail endpoint — packaging keeps the list with expand-row JSON; the Logs subtab on `/traces/:id` reuses this same endpoint with `?trace_id=X&limit=200`.

## §5 Frontend `/logs` page + cross-jump UX

**New route + nav.**

| | |
|---|---|
| Route | `/logs` (and `/logs?trace_id=X&span_id=Y&service=cartservice&severity=ERROR&…`) — URL is the source of truth for filter state |
| Sidebar | New "日志 / Logs" entry between "调用链 / Traces" and the disabled-for-now SLICE-3 entries |
| Page title | `i18n.logs.title` (zh-CN: "日志" / en-US: "Logs") |

**Components.**

| file | role |
|---|---|
| `views/LogsView.vue` (new) | List page; reads URL params → filter state; calls `api/logs.list()`; renders `<NDataTable>` with custom row |
| `components/LogRow.vue` (new) | One row: timestamp · severity badge · service · body (truncated 200 chars) · trace_id chip. Click row → expand; expanded view shows full body (JSON pretty-printed if `JSON.parse` succeeds) + `attributes` + `resource_attributes` |
| `components/SeverityBadge.vue` (new) | Maps `severity_text` → semantic color (DEBUG gray · INFO blue · WARN amber · ERROR red · FATAL purple). Uses existing openapm tokens |
| `api/logs.ts` (new) | `list(params)` + types; mirrors `api/traces.ts` |
| `views/TraceDetailView.vue` (edit) | Adds a third `<NTabs>` panel labeled "日志 / Logs" alongside Waterfall + JSON |
| `components/LogsPanel.vue` (new) | Reusable: takes `trace_id` + optional `span_id`; calls `api/logs.list` and renders rows. Used inside trace detail |

**Filter bar** (top of `/logs`, two-way bound to URL):
- Service: `<NSelect multiple>` (options for SLICE-2 = "auto-suggest from recent logs" using a small distinct-service helper, OR hardcoded "type your own"; final UX revisited when SLICE-3 lands the real `/api/v1/services` endpoint)
- Severity: `<NSelect multiple>` with the 5 fixed levels (DEBUG / INFO / WARN / ERROR / FATAL)
- Time range: `<NDatePicker type="datetimerange">` (default: last 1h → null/now)
- Trace ID: `<NInput>` (hex, optional)
- Span ID: `<NInput>` (hex, optional)
- Body contains: `<NInput>` (substring)
- "应用 / Apply" button rebuilds URL params + triggers fetch

**Cross-jump UX (per packaging B):**

1. **Trace detail → logs.** `TraceDetailView.vue` gains a "日志 / Logs" tab. Mounts `<LogsPanel :trace_id="route.params.trace_id" :span_id="selectedSpanId ?? undefined" />` which calls the same `api/logs.list()` with `?trace_id=X&limit=200`.
2. **Span-granular scoping.** Clicking a span in the SVG waterfall sets a local `selectedSpanId` ref. The Logs subtab reactive query re-runs with `?trace_id=X&span_id=Y`. A small "showing logs for span <name>" + "show all trace logs" toggle clears `selectedSpanId`. No new route — purely component state.
3. **Logs → trace.** Each `LogRow.vue` renders `trace_id` as `<RouterLink to="/traces/{{trace_id}}">`. `span_id` is rendered as a sibling chip; when present, the link is `to="/traces/{{trace_id}}?focus_span={{span_id}}"`. `TraceDetailView` reads `?focus_span` and pre-selects+scrolls to that span row in the waterfall.

**JSON body pretty-print** (per packaging B). On row expand: `try { JSON.parse(body) }`. On success: render with `<NCode :code="JSON.stringify(parsed, null, 2)" language="json">`. On failure: render as plain `<pre>`. No backend changes — purely client-side parse.

**i18n.** All labels in `frontend/src/locales/{zh-CN,en-US}.json` under `logs.*` (filter labels, severity names, table headers, "Logs" tab label, "showing logs for span" toggle).

**Tests.**
- `vitest`: `api/logs.test.ts` (URL build, error mapping), `SeverityBadge.test.ts` (color map), `LogRow.test.ts` (JSON pretty-print branch, link generation)
- `playwright`: `logs.spec.ts` — (a) seed + visit `/logs` sees row, (b) cross-tenant ACME-key-only sees ACME logs, (c) click `trace_id` chip → lands on `/traces/:id`, (d) `/traces/:id` → Logs subtab → see logs, (e) click span → Logs subtab narrows to that `span_id`

## §6 Log ingester service (`cmd/log-ingester`)

**Binary wiring** mirrors `cmd/ingester/main.go`: opens PG + CH, builds `auth.PGResolver`, builds `Metering` (PG-backed), builds `LogConsumer`, builds `OTLPLogReceiver`, starts admin server on `:8083`. Shutdown discipline: drain receiver before metering before admin (no metering loss on SIGTERM — proven by SLICE-1 ingester shutdown order).

### Package layout — refactor of `internal/ingest/`

The current `internal/ingest/` mixes trace-specific code (`spans.go`, `consume.go`, `traces_v1` INSERT) with shared scaffold (`admin.go`, `auth.go`, `host.go`, `metering.go`, `metrics.go`). For SLICE-2:

| package | files | role |
|---|---|---|
| `internal/ingestshared/` (new) | `admin.go`, `auth.go` (`extractBearer`), `host.go` (`nopHost`), `metering.go`, `metrics.go` (base counters) | Shared scaffold — both ingesters import |
| `internal/ingest/` (existing, trim) | `spans.go`, `consume.go`, `receiver.go`, `types.go`, `cross_tenant_test.go`, `spans_test.go` | **Trace-only** going forward |
| `internal/logingest/` (new) | `logs.go` (`logsToRows` + `bodyAsString`), `consume.go` (`LogConsumer.ConsumeLogs`), `receiver.go` (`NewOTLPLogReceiver` → `receiver.Logs`), `types.go` (`LogRow`), `logs_test.go`, `cross_tenant_test.go` | **Log-only** |

Refactor lands in two sub-tasks **before** any log-specific code is written, so trace tests stay green throughout. `chquery.Conn` + `chquery.Batch` unchanged. `metering.go` moves to shared; its enqueue path already takes `signal_type` (column name already in PG schema).

### `ConsumeLogs` pipeline

```go
func (c *LogConsumer) ConsumeLogs(ctx context.Context, td plog.Logs) error {
    // 1. Bearer extract → resolver → server-stamp tenant_id
    bearer, err := ingestshared.ExtractBearer(ctx)
    if err != nil {
        c.metrics.AuthMissing.Inc()
        return status.Error(codes.Unauthenticated, "missing bearer")
    }
    _, tn, err := c.resolver.ResolveBearer(ctx, bearer)
    if err != nil {
        c.metrics.AuthInvalid.Inc()
        return status.Error(codes.Unauthenticated, "invalid bearer")
    }
    ctx = auth.WithTenant(ctx, tn.ID, tn.Name)

    // 2. plog.Logs → []LogRow (drops SDK tenant.id, hex-encodes IDs, JSON-encodes Map bodies)
    rows := logsToRows(td)
    if len(rows) == 0 { return nil }

    // 3. chquery.Batch (pins tenant_id; first Append arg validated)
    batch, err := c.ch.PrepareBatch(ctx, insertLogsV1Stmt)
    if err != nil { /* counter + Internal */ }
    tidStr := tn.ID.String()
    for _, r := range rows {
        if err := batch.Append(
            tidStr, r.Ts, r.ObservedTs, r.Service, r.SeverityText, r.SeverityNumber,
            r.Body, r.TraceID, r.SpanID, r.TraceFlags, r.ResourceAttributes, r.Attributes,
        ); err != nil { _ = batch.Abort(); /* counter + Internal */ }
    }
    if err := batch.Send(); err != nil { /* counter + Internal */ }

    // 4. Best-effort metering (CH commit returns OK regardless)
    c.metering.Enqueue(MeteringEvent{TenantID: tn.ID, SignalType: "log", Count: len(rows)})
    return nil
}
```

`insertLogsV1Stmt`:
```sql
INSERT INTO logs_v1 (
  tenant_id, ts, observed_ts, service, severity_text, severity_number,
  body, trace_id, span_id, trace_flags, resource_attributes, attributes
) VALUES
```

### `logsToRows(td plog.Logs)` — OTLP → CH mapping

| OTLP field | CH column | notes |
|---|---|---|
| `LogRecord.Timestamp()` | `ts` | If zero, fall back to `ObservedTimestamp()` (SDK clock-skew safety) |
| `LogRecord.ObservedTimestamp()` | `observed_ts` | Default `time.Now()` if zero |
| `Resource.Attributes()["service.name"]` | `service` | `""` if missing |
| `LogRecord.SeverityText()` | `severity_text` | `""` if missing |
| `LogRecord.SeverityNumber()` | `severity_number` | `uint8(int(n))`; 0 if `SEVERITY_NUMBER_UNSPECIFIED` |
| `LogRecord.Body()` | `body` | `bodyAsString(v pcommon.Value)`: `v.AsString()` for primitives, `json.Marshal(v.AsRaw())` for Map/Slice |
| `LogRecord.TraceID()` / `SpanID()` | `trace_id` / `span_id` | hex-encode; all-zero → `""` |
| `LogRecord.Flags()` | `trace_flags` | `uint8(flags & 0xFF)` |
| `LogRecord.Attributes()` | `attributes` | `mapAttrs` (reused from `ingestshared`) |
| `ResourceLogs.Resource().Attributes()` | `resource_attributes` | `mapAttrs` minus `tenant.id` (server-stamping, same as traces) |

### Receiver wiring (`NewOTLPLogReceiver`)

Mirrors `NewOTLPReceiver` from SLICE-1:
- Same `otlpreceiver.NewFactory()`, same `Protocols.GRPC` + `Protocols.HTTP` config
- **`IncludeMetadata: true` on both transports from day 1** (SLICE-1 T10/T13 lesson — header-loss bug; do not replay)
- `HTTPConfig.LogsURLPath = "/v1/logs"` (the default; explicit for clarity)
- Returns `receiver.Logs`; built via `factory.CreateLogs(ctx, set, rcvrCfg, logConsumer)`

### Docker compose — new `log-ingester` service

```yaml
log-ingester:
  build: ../backend
  entrypoint: ["/usr/local/bin/log-ingester"]
  environment:
    DATABASE_URL: "postgres://openaiops:openaiops@postgres:5432/openaiops?sslmode=disable"
    CLICKHOUSE_DSN: "clickhouse://openaiops:openaiops@clickhouse:9000/openaiops"
    LOG_INGESTER_OTLP_GRPC_ADDR: "0.0.0.0:4327"
    LOG_INGESTER_OTLP_HTTP_ADDR: "0.0.0.0:4328"
    LOG_INGESTER_ADMIN_ADDR:     "0.0.0.0:8083"
  ports:
    - "127.0.0.1:${LOG_INGESTER_OTLP_GRPC_HOST_PORT:-4327}:4327"
    - "127.0.0.1:${LOG_INGESTER_OTLP_HTTP_HOST_PORT:-4328}:4328"
  depends_on:
    postgres:     { condition: service_healthy }
    clickhouse:   { condition: service_healthy }
    ch-migrate:   { condition: service_completed_successfully }
    migrate:      { condition: service_completed_successfully }
  healthcheck:
    test: ["CMD", "wget", "-qO-", "http://localhost:8083/livez"]
    interval: 5s
    timeout: 3s
    retries: 10
```

`backend/Dockerfile` — add `log-ingester` to the multi-target build (each `cmd/<x>` compiled separately per ADR-0003 multi-binary pattern).

### Seed helper

New `backend/cmd/seed-logs/main.go` (parallel to `cmd/seed-traces`) — uses `go.opentelemetry.io/otel/sdk/log` with OTLP exporter pointed at `LOG_INGESTER_OTLP_GRPC_HOST_PORT`, emits a small basket of records with varied severity + a known `trace_id`/`span_id` so the cross-jump UX has data. Makefile: `make seed-logs` + `make demo-logs` (alongside `seed-traces`/`demo-traces`).

## §7 Routing consolidation (drift D4 fix)

**Goal.** Caddy is the sole ingress for `/api/*`. The frontend container goes back to serving only static SPA assets. Adding a new query route in SLICE-3 (services, topology) becomes a one-line Caddy change with nothing to mirror.

**Changes:**

1. **`frontend/nginx.conf`** — delete both `location /api/v1/traces { ... }` and `location /api/ { ... }` blocks. Keep only the SPA static + `try_files $uri $uri/ /index.html` fallback. Net: the frontend container becomes a dumb static-asset server.

2. **`deploy/Caddyfile`** — already has the per-prefix split pattern. Adds **one** new line:
   ```caddyfile
   handle /api/v1/logs* {
       uri strip_prefix /api
       reverse_proxy query:8081
   }
   ```
   Order matters: `/api/v1/logs*` and `/api/v1/traces*` are listed before the catch-all `/api/*` → `gateway:8080`. Caddy's `handle` is mutually exclusive (first match wins).

3. **`frontend/vite.config.ts`** (dev only) — `server.proxy['/api']` repointed from `http://gateway:8080` to `https://localhost` (Caddy) with `secure: false, changeOrigin: true`. One source of truth for both `npm run dev` and prod.

4. **`playwright.config.ts`** — `baseURL` switches from `http://localhost:${FRONTEND_HOST_PORT:-13000}` to `https://localhost`, plus `use.ignoreHTTPSErrors: true` (Caddy uses `tls internal` for local dev — self-signed). Existing trace specs revalidate against Caddy with no test-body changes (paths are unchanged).

5. **New regression assertion** (one extra playwright test in `logs.spec.ts`): direct GET to `http://localhost:13000/api/v1/logs` returns 404 (or connection refused). This is the *architectural assertion* — it fails the moment someone adds `/api` routing back to nginx, preventing drift D4 from quietly returning.

6. **Docs.** `CLAUDE.md` "端口" block: change canonical URL to "Caddy :443 is the sole ingress; frontend container is static-only". `README.md` / `deploy/.env.example` updated.

7. **`FRONTEND_HOST_PORT` kept mapped** — useful for inspecting the raw SPA bundle without TLS. Harmless once `/api` returns 404 there.

**Drift bookkeeping.** Once shipped, `docs/lessons-learned-2026-05-24.md` D4 entry gets closed with the resolution; `docs/claude-progress.json` `known_drift` D4 moves to `recently_completed`.

**Symmetry check** for SLICE-3+: services / topology / alerts routes each add one `handle` block to `Caddyfile`. Zero nginx changes ever again.

## §8 Cross-tenant E2E (AC #8)

Go integration test, build tag `integration`, at `backend/internal/logingest/cross_tenant_test.go`. Reuses the `chtest` shared package introduced in SLICE-1. Mirrors SLICE-1 AC #8 structure: ACME + Beta tenant seeds, dockertest-spun ephemeral CH + PG, in-process log ingester listening on ephemeral ports.

**8 sub-assertions:**

| # | scenario | expected |
|---|---|---|
| 1 | Tenant A ingests N log records via OTLP/gRPC `:4327` with `key-acme` | 200 OK; CH `SELECT count() FROM logs_v1 WHERE tenant_id=<A>` returns N |
| 2 | Tenant B queries `/api/v1/logs` with `key-beta` | empty `items`, `has_more=false` (cross-tenant **read** denied — chquery + Row Policy) |
| 3 | Tenant B queries `/api/v1/logs?trace_id=<A's trace_id>` | empty `items` (cross-jump cannot leak) |
| 4 | Tenant A queries `/api/v1/logs?trace_id=<A's trace_id>` | N rows (own correlation works) |
| 5 | Tenant A queries `/api/v1/logs?span_id=<A's span_id>` | expected subset (span-granular scoping works per packaging B) |
| 6 | OTLP/gRPC `:4327` with no `Authorization` metadata | `codes.Unauthenticated`; counter `log_ingester_auth_missing_total++` |
| 7 | OTLP/HTTP `:4328 /v1/logs` with no `Authorization` header | HTTP 401 (HTTP-transport regression — SLICE-1 T13 lesson, prevents header-loss bug repeat) |
| 8 | `/api/v1/logs` with garbage Bearer | HTTP 401 |

**Sub-assertion 7 is load-bearing.** SLICE-1 T13 caught `IncludeMetadata=true` missing on HTTP transport silently losing headers — we assert it on day 1 for logs.

## §9 CI matrix update

| job | added |
|---|---|
| `backend-unit` | new `internal/logingest/*_test.go` (unit), new `internal/query/logs_*_test.go` (handler + repo) |
| `backend-integration` (`-tags=integration -timeout=240s`) | new `internal/logingest/cross_tenant_test.go` (8 sub-assertions); reuses existing `chtest` |
| `frontend-unit` (vitest) | new `api/logs.test.ts`, `components/SeverityBadge.test.ts`, `components/LogRow.test.ts` |
| `e2e` (Playwright) | new `make seed-logs` helper run before; new `tests/e2e/logs.spec.ts` (5 cases per §5 + drift D4 regression assertion); existing trace specs revalidate against Caddy `baseURL` |
| `lint-ch` | unchanged — already covers new `internal/logingest/` + `internal/query/logs_repo.go` automatically (denies bare `clickhouse-go` outside `internal/chquery`) |

## §10 Out of scope / known limitations

**Out of scope:**
- Log retention / TTL — admin concern; SLICE-3+ when tenant settings UI lands
- Full-text body search via `tokenbf_v1` / `ngrambf_v1` index — revisit when query patterns are real
- Log↔trace **bidirectional graph** UI — we ship cross-jump links + tabs, not a unified timeline view
- OTLP log "events"/structured normalization — treated as plain `body` + `attributes` in v1
- Log-based metrics / extracted spans — post-MVP
- Sampling decisions on logs — ingest 100% in v1
- Asynchronous insert / column-mode batch on `chquery.Batch` — add when throughput data demands
- Cursor pagination on `/api/v1/logs` — offset paging ships with documented streaming-drift limitation, symmetric to SLICE-1
- Proactive 501 stubs for `/api/v1/services|topology|alerts` — future slices add their own one-line Caddy handle

**Known limitations (carried into evidence):**
- `body_contains` does a full scan within partition selection (no text index). Slow on 100M+ rows; acceptable at MVP.
- Logs are typically 10× trace volume; the `(tenant_id, toYYYYMMDD(ts))` partition strategy is the same as traces — partition count pressure will surface earlier on logs. Revisit `toYYYYMM` partitioning in SLICE-3 if MergeTree part counts grow problematic.
- `service` filter on `/logs` page uses a temporary "type your own" or "auto-suggest from recent" until SLICE-3 lands the real `/api/v1/services` endpoint.
- Default `ts_to = now + 1s` (drift D1 mitigation carried over from SLICE-1 trace list). Cleaner semantics (`ts <=` instead of `ts <`) deferred to a single follow-up across both signals.

## §11 References

- Spec: this file (`docs/specs/2026-05-26-slice-2-log-design.md`)
- Predecessor: `docs/specs/2026-05-25-slice-1-trace-design.md` (SLICE-1 — closed 2026-05-26)
- ADRs: `0001-initial-architecture.md` (multi-tenant Row Policy, tenant-first ORDER BY), `0002-clickhouse-schema-migrations.md` (forward-only `_schema_migrations`), `0003-query-api-deployment-shape.md` (multi-binary single-image)
- Lessons learned: `docs/lessons-learned-2026-05-24.md` (D4 closes here)
- Progress: `docs/claude-progress.json` (SLICE-2 enters `current_focus` on start; `known_drift` D4 closes)
- Project rules: `CLAUDE.md` "多租户" + "二进制 + 路由划分" + "CH 迁移" blocks
