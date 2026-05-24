# ADR-0002: ClickHouse schema migration mechanism

- **Status**: Accepted
- **Date**: 2026-05-24
- **Deciders**: @huangbaixun
- **Tracks**: PRE-1 (Slice 1 blocker)
- **Supersedes**: —

## Context

Slice 0 stood up a ClickHouse 23.12 container with empty database `openaiops`. Slice 1 will be the first slice to write CH tables (`traces_v1`). Before we write any DDL, we must decide *how* CH schema gets created and evolved — analogous to PG's `goose`, but for CH.

Two candidates evaluated:

### Option A — `docker-entrypoint-initdb.d/`

The official `clickhouse-server` image runs every `.sql` / `.sh` mounted at `/docker-entrypoint-initdb.d/` exactly once, on **fresh container init only**. The named volume `chdata` already persists data across restarts, so a marker file inside the volume suppresses re-execution.

- **Pros**: Zero new infra. Just mount and forget. Mirrors how PG is often initialized.
- **Cons**:
  - Only runs on a virgin data dir. Adding a second migration later means we have **no story** for evolving an existing cluster.
  - Mixing initialization-time DDL (in compose) with future migrations (somewhere else) creates two parallel sources of schema truth.
  - Doesn't support a tracking table; no way to know what's already applied if someone partially re-runs.

### Option B — Dedicated `ch-migrate` service (chosen)

Symmetric to the existing PG `migrate` service. A short-lived container runs a script that:

1. Waits for CH to be reachable.
2. Creates a `_schema_migrations` tracking table in CH (if not present).
3. Walks `backend/ch-migrations/*.sql` in lex order.
4. Skips any version already in `_schema_migrations`; applies the rest via `clickhouse-client --multiquery --queries-file`.
5. Inserts each applied version into the tracking table.

- **Pros**: Same mental model as PG goose. Same Makefile shape. Runs idempotently on every `make up`. Supports running against a *long-lived* cluster (the whole point). Trivial to upgrade to a real sequencer (e.g., a goose-like binary) when we need DOWN migrations.
- **Cons**:
  - Slightly heavier — one extra short-lived container per `compose up`.
  - Forward-only at v1. No DOWN. We accept this for v0.1 because (a) we have *zero* SLICE-1 ALTER scenarios on day 1, (b) CH rarely needs rollback (it's the warehouse, not the source of truth — for catastrophic errors we drop the table and re-ingest).
  - Tracking table uses plain `MergeTree`; if the runner is SIGKILLed *between* applying DDL and inserting the version row, the next run re-applies the migration. Survivable for `CREATE TABLE IF NOT EXISTS` (idempotent); not survivable for `ALTER TABLE ADD COLUMN`. Mitigation: until we have an ALTER, we don't care; when we do, we swap the runner.

## Decision

**Option B.** Implementation lives in `deploy/ch-migrate.sh` + `backend/ch-migrations/` + `ch-migrate` service entry in `deploy/docker-compose.yml`. Tracking table is `_schema_migrations(version String, applied_at DateTime DEFAULT now()) ENGINE = MergeTree ORDER BY version`.

### Naming + file format

- Files: `backend/ch-migrations/YYYYMMDDHHMMSS_<verb>.sql` (same shape as PG goose).
- Content: plain CH SQL. **No `-- +goose Up` / `Down` pragmas** — we run our own loop, not goose.
- One file = one logical migration = one tracking-table row.
- Multiple statements per file are allowed (`--multiquery` flag). Semicolon-separated.

### Service shape

- Image: `clickhouse/clickhouse-server:23.12-alpine` (reuse, already cached by the `clickhouse` service).
- Entrypoint overridden to `/bin/sh /run.sh`.
- `depends_on: clickhouse { condition: service_healthy }`.
- `restart: "no"`. Exit 0 on success, exit 1 on any SQL failure (set -e).
- Future ingester/query services that touch CH must `depends_on: ch-migrate { condition: service_completed_successfully }`. SLICE-0 has no such service yet; this dependency edge gets added in SLICE-1.

### Tenant-isolation interaction (ADR-0001 §3.3)

The ch-migrate runner is **trusted infrastructure** — it executes DDL with the admin user. It does NOT go through `MustTenantScope` (PRE-3). Tenant isolation applies to **runtime queries** (read/write of tenant data), not schema management. The migration files themselves MUST honor the rule "every CH table starts with `tenant_id LowCardinality(String)` as ORDER BY prefix" — enforced by SLICE-1 spec compliance review, not by the runner.

## Consequences

- New convention added to `CLAUDE.md` "活规则 / CH 迁移" block.
- `Makefile` gets a `migrate-ch-up` target for ad-hoc use against a running cluster.
- The `chdata` volume is now meaningful between restarts but **safe to nuke** — `ch-migrate` will reapply everything on a fresh volume. We do NOT seed CH data from a SQL file (analogous to PG seed.sql is OK — seed is application data, not schema).
- Forward-only is documented as a known limitation. When the first ALTER lands (likely SLICE-1+1 if `traces_v1` needs a column added), revisit this ADR — at that point either (a) add a manual "apply once, then mark applied" affordance, or (b) replace the runner with a real sequencer.
- **What this does NOT block**: SLICE-1 can start writing the first migration file (`YYYYMMDDHHMMSS_create_traces_v1.sql`) the moment its design doc lands. PRE-1 is now resolved.

## Verification

After this ADR ships:

1. `docker compose -f deploy/docker-compose.yml config` parses cleanly with the new service.
2. `make up` brings ch-migrate to `exited (0)` with no SQL files present (no-op success).
3. Adding a single dummy `.sql` (e.g. `CREATE TABLE _smoke (x UInt8) ENGINE = MergeTree ORDER BY tuple()`) and re-running `make up` results in: (i) the table created, (ii) one row in `_schema_migrations`, (iii) a second `make up` shows "skip ... (already applied)" and exits 0.

(1) is automated below. (2) and (3) are manual smoke checks; SLICE-1 will add an automated CI test that walks the full migration set against a fresh CH container.
