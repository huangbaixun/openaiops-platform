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
