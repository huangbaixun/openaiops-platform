-- traces_v1: SLICE-1 OTLP span landing table. See docs/specs/2026-05-25-slice-1-trace-design.md §3.
CREATE TABLE traces_v1 (
    tenant_id           LowCardinality(String),
    trace_id            String CODEC(ZSTD(1)),
    span_id             String CODEC(ZSTD(1)),
    parent_span_id      String CODEC(ZSTD(1)),
    service             LowCardinality(String),
    operation           LowCardinality(String),
    ts                  DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    duration            UInt64 CODEC(T64, LZ4),
    status              LowCardinality(String),
    span_kind           LowCardinality(String),
    resource_attributes Map(LowCardinality(String), String),
    attributes          Map(LowCardinality(String), String),
    INDEX idx_trace_id trace_id TYPE bloom_filter(0.01) GRANULARITY 4
) ENGINE = MergeTree
PARTITION BY (tenant_id, toYYYYMMDD(ts))
ORDER BY (tenant_id, service, ts)
SETTINGS index_granularity = 8192;

CREATE ROW POLICY tenant_isolation_traces_v1 ON traces_v1
    USING tenant_id = getSetting('custom_tenant_id')
    TO openaiops;
