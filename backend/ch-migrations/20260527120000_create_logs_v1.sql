-- logs_v1: SLICE-2 OTLP log landing table. See docs/specs/2026-05-26-slice-2-log-design.md §3.
-- IF NOT EXISTS on both DDLs for ch-migrate runner replay-safety (SLICE-1 T1 lesson).
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
PARTITION BY (tenant_id, toYYYYMMDD(ts))
ORDER BY (tenant_id, service, ts)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_logs_v1 ON logs_v1
    USING tenant_id = getSetting('custom_tenant_id') TO openaiops;
