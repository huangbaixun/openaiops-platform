-- service_stats_v1: SLICE-3 per-service RED aggregation (1min buckets, split by span_kind).
-- IF NOT EXISTS on both DDLs for ch-migrate runner replay-safety (SLICE-1 T1 lesson).
CREATE TABLE IF NOT EXISTS service_stats_v1 (
    tenant_id     LowCardinality(String),
    ts_bucket     DateTime CODEC(Delta, ZSTD(1)),
    service       LowCardinality(String),
    span_kind     LowCardinality(String),
    calls         UInt64 CODEC(T64, LZ4),
    errors        UInt64 CODEC(T64, LZ4),
    p95_duration  UInt64 CODEC(T64, LZ4)
) ENGINE = ReplacingMergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts_bucket))
ORDER BY (tenant_id, ts_bucket, service, span_kind)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_service_stats_v1 ON service_stats_v1
    USING tenant_id = getSetting('custom_tenant_id') TO openaiops;
