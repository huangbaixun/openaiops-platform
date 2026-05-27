-- topology_edges_v1: SLICE-3 service-to-service edge aggregation (1min buckets).
-- ReplacingMergeTree + queries with FINAL: topo-engine re-runs cleanly without
-- PG-tracked exactly-once complexity. IF NOT EXISTS on both DDLs for ch-migrate
-- runner replay-safety (SLICE-1 T1 lesson).
CREATE TABLE IF NOT EXISTS topology_edges_v1 (
    tenant_id      LowCardinality(String),
    ts_bucket      DateTime CODEC(Delta, ZSTD(1)),
    caller_service LowCardinality(String),
    caller_kind    LowCardinality(String),
    callee_service LowCardinality(String),
    callee_kind    LowCardinality(String),
    calls          UInt64 CODEC(T64, LZ4),
    errors         UInt64 CODEC(T64, LZ4),
    p95_duration   UInt64 CODEC(T64, LZ4)
) ENGINE = ReplacingMergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts_bucket))
ORDER BY (tenant_id, ts_bucket, caller_service, caller_kind, callee_service, callee_kind)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_topology_edges_v1 ON topology_edges_v1
    USING tenant_id = getSetting('custom_tenant_id')
    TO openaiops;
