-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE tenants (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT         NOT NULL UNIQUE,
    plan          TEXT         NOT NULL DEFAULT 'free',
    rate_limit_per_min INTEGER NOT NULL DEFAULT 600,
    data_retention_days INTEGER NOT NULL DEFAULT 30,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenants_name ON tenants(name);

-- +goose Down
DROP TABLE IF EXISTS tenants;
