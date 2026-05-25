#!/bin/sh
# ClickHouse forward-only migration runner. See docs/decisions/0002-clickhouse-schema-migrations.md.
set -eu

# Reject DB names that aren't safe to interpolate into raw SQL.
case "${CH_DATABASE:-openaiops}" in
    *[!A-Za-z0-9_]* | "")
        echo "[ch-migrate] FATAL: CH_DATABASE must match [A-Za-z0-9_]+" >&2
        exit 1
        ;;
esac

CH_HOST="${CH_HOST:-clickhouse}"
CH_PORT="${CH_PORT:-9000}"
CH_USER="${CH_USER:-openaiops}"
CH_PASSWORD="${CH_PASSWORD:-openaiops}"
CH_DATABASE="${CH_DATABASE:-openaiops}"
MIG_DIR="${MIG_DIR:-/migrations}"
WAIT_SECS="${WAIT_SECS:-30}"

ch() {
    clickhouse-client \
        --host "$CH_HOST" --port "$CH_PORT" \
        --user "$CH_USER" --password "$CH_PASSWORD" \
        --database "$CH_DATABASE" \
        "$@"
}

# Connect to the "default" DB for readiness + bootstrap CREATE DATABASE.
# The 23.12-alpine entrypoint's CLICKHOUSE_DB init is unreliable (its
# DATABASE_ALREADY_EXISTS / RUN_INITDB_SCRIPTS branch inverts on a fresh
# volume — same workaround used by chquery's smoke test).
ch_default() {
    clickhouse-client \
        --host "$CH_HOST" --port "$CH_PORT" \
        --user "$CH_USER" --password "$CH_PASSWORD" \
        --database default \
        "$@"
}

echo "[ch-migrate] waiting for clickhouse @ $CH_HOST:$CH_PORT (up to ${WAIT_SECS}s)..."
i=0
while [ "$i" -lt "$WAIT_SECS" ]; do
    if ch_default --query "SELECT 1" >/dev/null 2>&1; then
        echo "[ch-migrate] clickhouse ready"
        break
    fi
    i=$((i + 1))
    sleep 1
done
if ! ch_default --query "SELECT 1" >/dev/null 2>&1; then
    echo "[ch-migrate] FATAL: clickhouse not reachable after ${WAIT_SECS}s" >&2
    exit 1
fi

# Ensure target database exists before any per-DB query runs.
ch_default --query "CREATE DATABASE IF NOT EXISTS $CH_DATABASE"

ch --query "
CREATE TABLE IF NOT EXISTS _schema_migrations (
    version String,
    applied_at DateTime DEFAULT now()
) ENGINE = MergeTree
ORDER BY version
"

applied=$(ch --query "SELECT version FROM _schema_migrations ORDER BY version" --format=TabSeparated || echo "")

if [ ! -d "$MIG_DIR" ]; then
    echo "[ch-migrate] no migrations dir at $MIG_DIR — nothing to do"
    exit 0
fi

applied_count=0
skipped_count=0
total=0
for f in $(find "$MIG_DIR" -maxdepth 1 -name '*.sql' -type f 2>/dev/null | sort); do
    total=$((total + 1))
    version=$(basename "$f" .sql)
    if printf '%s\n' "$applied" | grep -qx "$version"; then
        echo "[ch-migrate] skip   $version (already applied)"
        skipped_count=$((skipped_count + 1))
        continue
    fi
    echo "[ch-migrate] apply  $version"
    # Atomic apply+record: send the migration body and the tracking-row insert
    # in a single ClickHouse RPC over stdin. If we crash between the two,
    # we'd brick the runner on the next run (replay → "already exists").
    # Strip trailing whitespace and any trailing ';'s from the body so we can
    # re-add exactly one ';' before appending the INSERT statement.
    {
        awk '
            { lines[NR] = $0 }
            END {
                # Concatenate all lines back, then trim trailing whitespace + ;.
                out = ""
                for (i = 1; i <= NR; i++) {
                    out = out (i > 1 ? "\n" : "") lines[i]
                }
                sub(/[[:space:];]+$/, "", out)
                printf "%s", out
            }
        ' "$f"
        printf ';\nINSERT INTO _schema_migrations (version) VALUES ('"'"'%s'"'"');\n' "$version"
    } | ch --multiquery
    applied_count=$((applied_count + 1))
done

if [ "$total" -eq 0 ]; then
    echo "[ch-migrate] no .sql files in $MIG_DIR — nothing to do (this is OK pre-SLICE-1)"
fi
echo "[ch-migrate] done. applied=$applied_count skipped=$skipped_count total=$total"
