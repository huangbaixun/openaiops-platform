# Architecture snapshot

> Current actual state as of 2026-05-24, post SLICE-0 + PRE-1/PRE-2 decisions.
> For the full design rationale + Phase 1/2 roadmap, read [ADR-0001](decisions/0001-initial-architecture.md).
> For deviations from this snapshot, read the relevant ADR ([0002](decisions/0002-clickhouse-schema-migrations.md), [0003](decisions/0003-query-api-deployment-shape.md)) or open `docs/decisions/README.md`.

## Stack at a glance

```
┌─ Frontend ────────────────────────────────────────────────────┐
│  Vue 3 + Vite + Pinia + Naive UI + vue-i18n (zh-CN/en-US)     │
│  nginx static + same-origin /api proxy → Caddy                │ :3000
└─────────────────────────┬─────────────────────────────────────┘
                          │ Bearer API key
┌─────────────────────────┴─────────────────────────────────────┐
│  Caddy (reverse proxy, path-based routing)                    │ :80
│  /api/v1/traces*│logs*│services*│topology*   → query:8081     │
│  /api/*                                       → gateway:8080  │
│  (catch-all)                                  → frontend:80   │
└──────┬───────────────────────────────────────────────┬────────┘
       │                                               │
┌──────┴────────────┐                       ┌──────────┴────────┐
│ gateway (Go 1.25) │                       │ query (Go, new    │
│ chi v5 + pgx v5   │                       │ at SLICE-1)       │
│ /healthz /livez   │                       │ /api/v1/traces*…  │
│ /api/v1/admin*    │                       │ /healthz /livez   │
│ /api/v1/metering* │                       │                   │
└──────┬────────────┘                       └─────┬────────┬────┘
       │ PG (auth + admin)                        │ PG     │ CH
       ▼                                          ▼ (auth) ▼ (data)
┌────────────────────┐  ┌───────┐  ┌─────────────────────────────┐
│ PostgreSQL 16      │  │ Redis │  │ ClickHouse 23.12            │
│ tenants            │  │ (rate │  │ _schema_migrations          │
│ api_keys (bcrypt)  │  │ limit │  │ traces_v1   (SLICE-1)       │
│ metering_events    │  │ etc.) │  │ logs_v1     (SLICE-2)       │
│ + goose migrations │  │       │  │ + Row Policies (PRE-3)      │
└────────────────────┘  └───────┘  └────────────┬────────────────┘
                                                ▲ OTLP gRPC :4317
                                                │      HTTP :4318
                                  ┌─────────────┴────────────────┐
                                  │ OTel Collector (debug exporter
                                  │ only at SLICE-0; SLICE-1 adds
                                  │ tenant.id processor + ingester)
                                  └──────────────────────────────┘
```

Ports: 8080 gateway · 8081 query (SLICE-1) · 3000 frontend · 4317/4318 OTLP. All host-bind to 127.0.0.1; override via `deploy/.env.local`.

## Repo layout — where to look

```
openaiops-platform/
├── backend/
│   ├── cmd/
│   │   ├── gateway/        write + admin binary (SLICE-0)
│   │   ├── query/          read binary (PRE-2 decided, SLICE-1 implements)
│   │   └── seed-hash/      one-shot util to bcrypt-hash seed keys
│   ├── internal/
│   │   ├── auth/           Bearer middleware + ctx tenant injection
│   │   ├── apikey/         PG resolver, bcrypt-verifies on each req
│   │   ├── tenant/         domain types
│   │   ├── config/         env loading (gains LoadQuery() at SLICE-1)
│   │   ├── httpsrv/        graceful-shutdown template (gains Run() at SLICE-1)
│   │   ├── chquery/        MustTenantScope helper (PRE-3 will create)
│   │   └── query/          read handlers (SLICE-1 will create)
│   ├── migrations/         PG goose migrations (3 tables, SLICE-0)
│   └── ch-migrations/      CH migrations runner — see ADR-0002
├── frontend/               Vue 3 SPA (Login + Home + AppLayout shell)
├── deploy/
│   ├── docker-compose.yml  full stack
│   ├── ch-migrate.sh       CH migration runner (PRE-1)
│   ├── Caddyfile           path-based reverse proxy (will gain query routes at SLICE-1)
│   ├── otel-collector-config.yaml
│   └── seed.sql            dev tenants + API keys (plaintext OK, dev-only)
└── docs/
    ├── architecture.md         this file
    ├── decisions/              ADRs (index in decisions/README.md)
    ├── specs/                  brainstorming output → ADR back-references
    ├── plans/                  implementation plans
    ├── lessons-learned-*.md    sediment from slice retros
    ├── claude-progress.json    cross-session focus + open_tasks ledger
    └── agent-telemetry.jsonl   auto-generated, gitignore-eligible
```

`features.json` lives at repo root, not under `docs/` — convention from harness:init.

## Data flow — current vs SLICE-1

**Current (SLICE-0):**

1. Browser → nginx (same-origin) → Caddy → gateway → PG (auth) → JSON response.
2. No CH path is wired. OTel Collector is up but only debug-exports.

**SLICE-1 target:**

1. OTel demo (hot-r.o.d.) → Collector (with tenant.id processor) → Go ingester → CH `traces_v1` (tenant-scoped INSERT via `chquery.MustTenantScope`).
2. Browser → Caddy → query binary → CH SELECT (tenant-scoped via same helper + CH Row Policy as defense-in-depth) → JSON waterfall → /traces page.
3. Per-batch ingest writes a row to PG `metering_events` (`signal_type='trace', count=spans`).

## Multi-tenant invariants (ADR-0001 §3.3 — load-bearing)

Every CH/PG query MUST go through a builder that injects `tenant_id`. The chosen API is `chquery.MustTenantScope(ctx, base, args...) (string, []any)` — fixed by ADR-0003, implemented by PRE-3. Three layers of defense:

1. **Builder layer** — `MustTenantScope` panics if ctx has no tenant; injects `WHERE tenant_id = ?`. Programmer error caught at runtime.
2. **CH Row Policy** — DB-side filter that activates per user/role. Survives even if a future helper-bypass slips through.
3. **Cross-tenant reverse E2E** — tenant A writes / tenant B reads → MUST return 0 rows. CI gate.

Layer 1+2 land with PRE-3. Layer 3 is SLICE-1 AC #8.

## Currently open prerequisites

| ID | Title | Status |
|---|---|---|
| PRE-1 | ClickHouse schema migration mechanism | ✅ Resolved (ADR-0002) |
| PRE-2 | Query API deployment shape | ✅ Resolved (ADR-0003) |
| PRE-3 | MustTenantScope + Row Policy + reverse E2E | 🟠 Open — last gate before SLICE-1 code |

Live state: `docs/claude-progress.json`.
