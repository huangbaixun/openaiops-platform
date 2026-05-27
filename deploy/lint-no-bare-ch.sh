#!/bin/sh
# Forbid direct clickhouse-go usage anywhere except internal/chquery itself.
# Enforces ADR-0001 §3.3 layer 1 at build time.
set -eu

ROOT="${ROOT:-backend}"
SCAN_DIRS="
${ROOT}/internal/query
${ROOT}/internal/ingest
${ROOT}/internal/logingest
${ROOT}/internal/ingestshared
${ROOT}/cmd
"

violations=0
for dir in $SCAN_DIRS; do
    if [ ! -d "$dir" ]; then continue; fi
    # Find any import of clickhouse-go OR direct method calls bypassing chquery.
    hits=$(grep -rn -E '("github.com/ClickHouse/clickhouse-go|\bclickhouse\.Open\(|\bclickhouse\.Context\()' "$dir" 2>/dev/null || true)
    if [ -n "$hits" ]; then
        echo "FAIL: $dir contains direct clickhouse-go usage (must go through chquery.Conn):" >&2
        echo "$hits" >&2
        violations=$((violations + 1))
    fi
done

if [ "$violations" -gt 0 ]; then
    echo "" >&2
    echo "Violation: code outside backend/internal/chquery/ must NOT import" >&2
    echo "clickhouse-go directly. Use chquery.Connect / chquery.Conn instead." >&2
    echo "See ADR-0001 §3.3 + ADR-0003." >&2
    exit 1
fi

# Rule 2: chquery.AdminConn may only be constructed under the topo-engine
# subsystem — the internal/topoengine/ package and its cmd/topo-engine/ binary
# (the only subsystem authorized to bypass MustTenantScope). _test.go files are
# allowed anywhere — they exercise the AdminConn surface itself.
# See SLICE-3 spec §2 (Tenant trust — topo-engine is an internal service).
BAD_ADMIN=$(grep -rn "chquery\.NewAdminConn\b" "${ROOT}/" --include="*.go" 2>/dev/null \
    | grep -v "_test\.go:" \
    | grep -v "^${ROOT}/internal/chquery/" \
    | grep -v "^${ROOT}/internal/topoengine/" \
    | grep -v "^${ROOT}/cmd/topo-engine/" \
    || true)

if [ -n "$BAD_ADMIN" ]; then
    echo "FAIL: chquery.NewAdminConn constructed outside internal/topoengine/ or cmd/topo-engine/:" >&2
    echo "$BAD_ADMIN" >&2
    echo "" >&2
    echo "Violation: AdminConn bypasses MustTenantScope. Only the topo-engine" >&2
    echo "subsystem (internal/topoengine/ or cmd/topo-engine/) may construct it." >&2
    echo "See SLICE-3 spec §2." >&2
    exit 1
fi

echo "lint-no-bare-ch: OK ($SCAN_DIRS clean; AdminConn confined to topo-engine subsystem)"
