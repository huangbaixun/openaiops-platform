-- +goose Up
CREATE TABLE domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tenants ADD COLUMN domain_id   UUID NULL REFERENCES domains(id) ON DELETE SET NULL;
ALTER TABLE tenants ADD COLUMN environment TEXT NULL;
CREATE INDEX idx_tenants_domain ON tenants(domain_id);

CREATE TABLE audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_key_id  UUID NULL,
    action        TEXT NOT NULL,
    from_tenant_id UUID NULL,
    to_tenant_id   UUID NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_tenant;
DROP TABLE IF EXISTS audit_log;
DROP INDEX IF EXISTS idx_tenants_domain;
ALTER TABLE tenants DROP COLUMN IF EXISTS environment;
ALTER TABLE tenants DROP COLUMN IF EXISTS domain_id;
DROP TABLE IF EXISTS domains;
