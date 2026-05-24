-- +goose Up
CREATE TABLE metering_events (
    id            BIGSERIAL    PRIMARY KEY,
    tenant_id     UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    signal_type   TEXT         NOT NULL CHECK (signal_type IN ('trace','log','metric')),
    count         BIGINT       NOT NULL DEFAULT 1,
    ts            TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_metering_tenant_ts ON metering_events(tenant_id, ts DESC);

-- +goose Down
DROP TABLE IF EXISTS metering_events;
