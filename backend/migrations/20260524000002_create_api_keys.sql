-- +goose Up
CREATE TABLE api_keys (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          TEXT         NOT NULL,
    hashed_key    TEXT         NOT NULL UNIQUE,
    scope         TEXT         NOT NULL DEFAULT 'read-write',
    revoked_at    TIMESTAMPTZ  NULL,
    last_used_at  TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_api_keys_active ON api_keys(hashed_key) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS api_keys;
