-- +goose Up
CREATE TABLE annotations (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target_type     TEXT         NOT NULL CHECK (target_type IN ('trace','service')),
    target_id       TEXT         NOT NULL,
    kind            TEXT         NOT NULL,
    payload         JSONB        NOT NULL,
    ts              TIMESTAMPTZ  NOT NULL,
    idempotency_key TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_annotations_lookup ON annotations(tenant_id, target_type, target_id);
CREATE UNIQUE INDEX uq_annotations_idem ON annotations(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS annotations;
