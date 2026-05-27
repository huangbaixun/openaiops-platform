# ClickHouse migrations

Forward-only migration files for ClickHouse. Runner: `deploy/ch-migrate.sh`. Decision: `docs/decisions/0002-clickhouse-schema-migrations.md`.

## Convention

- File name: `YYYYMMDDHHMMSS_<verb>_<thing>.sql` (same shape as `backend/migrations/` PG goose files).
- **No goose pragmas.** Plain CH SQL. Multiple statements per file allowed (semicolon-separated).
- One file = one logical migration = one row in CH's `_schema_migrations` tracking table.
- **First column of every business table MUST be `tenant_id LowCardinality(String)`** and `ORDER BY (tenant_id, ...)` MUST start with `tenant_id` (ADR-0001 §3.3). Enforced at spec-review time.
- Forward-only: no DOWN. If you need to undo a migration in dev, nuke the `chdata` volume (`docker compose down -v`) and rerun `make up`.

## Ordering

Files are applied in lex (sort) order. Use a UTC timestamp prefix that monotonically increases. Conflict resolution between branches: pick a later timestamp for whichever lands second.

## Adding a new migration

```
touch backend/ch-migrations/$(date -u +%Y%m%d%H%M%S)_<verb>_<thing>.sql
```

Then either `make up` (full stack) or `make migrate-ch-up` (just the runner against an already-running CH).

## Row Policy template (multi-tenant enforcement)

Every business CH table created by a migration MUST have a Row Policy attached.
This is layer 2 of ADR-0001 §3.3 defense-in-depth. The policy filters rows via
the session setting `custom_tenant_id`, which `chquery.Conn` injects on every
Query/Exec.

Template — paste into the migration file that creates the table:

```sql
CREATE ROW POLICY IF NOT EXISTS tenant_isolation_<table_name> ON <table_name>
    USING tenant_id = getSetting('custom_tenant_id')
    TO openaiops;
```

Example for the SLICE-1 `traces_v1` migration:

```sql
CREATE TABLE IF NOT EXISTS traces_v1 (
    tenant_id LowCardinality(String),
    -- ... other columns ...
) ENGINE = MergeTree ORDER BY (tenant_id, service, ts);

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_traces_v1 ON traces_v1
    USING tenant_id = getSetting('custom_tenant_id')
    TO openaiops;
```

Notes:

- Setting name MUST be prefixed `custom_` (CH security rule).
- Policy must be `TO openaiops` (the app user). Other roles bypass; do NOT grant.
- The forward-only ch-migrate runner replays the policy if you nuke the volume,
  so policies are not stored elsewhere — they live with the table that needs them.
- The CH server itself needs the `custom_` prefix allowed at config level —
  see `deploy/clickhouse-custom-settings.xml` (bind-mounted by docker-compose).
  Without that config, layer 2 silently fails.
