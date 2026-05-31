-- Seed two tenants + one API key each for Slice 0 smoke tests, plus one
-- service:ai cross-tenant key for PLATFORM-ASK-1, plus a demo domain with
-- two env-tagged tenants + a domain-scoped key for PLATFORM-MT-1.
-- Hashes generated via `go run ./backend/cmd/seed-hash <plaintext>` (bcrypt cost 10).
-- Plaintexts test-key-acme / test-key-beta / test-key-ai / test-key-domain are intentionally PUBLIC dev-only.

TRUNCATE audit_log, api_keys, tenants, domains, metering_events RESTART IDENTITY CASCADE;

INSERT INTO tenants(id, name, plan) VALUES
  ('11111111-1111-1111-1111-111111111111', 'acme', 'free'),
  ('22222222-2222-2222-2222-222222222222', 'beta', 'free'),
  ('99999999-9999-9999-9999-999999999999', 'ai-svc', 'free');

INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES
  ('11111111-1111-1111-1111-111111111111', 'acme-primary', '$2a$10$/1JEzjuI8M5EVtVVVqMFPuZ3E.WZu1tJBCpiRsJf8PFrWw.ZbVCVG', 'read-write'),
  ('22222222-2222-2222-2222-222222222222', 'beta-primary', '$2a$10$dXNGJGtlwM11osqvLPLcceGRNcD5JKJ/FkNdm2TSETTJjJG4wQj.y', 'read-write'),
  -- service:ai key (plaintext test-key-ai, dev-only): may target any tenant via the X-Tenant-Id header (PLATFORM-ASK-1).
  ('99999999-9999-9999-9999-999999999999', 'ai-service', '$2a$10$tzDv/m3Wv9hiej64BT/tfOev1nJxCQS3d8OiOE52taSJg5O37z3ai', 'service:ai');

-- PLATFORM-MT-1: a demo domain with two env-tagged tenants + a domain-scoped key.
INSERT INTO domains(id, name) VALUES
  ('d0000000-0000-0000-0000-000000000001', 'demo-domain');

INSERT INTO tenants(id, name, plan, domain_id, environment) VALUES
  ('33333333-3333-3333-3333-333333333333', 'shop-prod',    'free', 'd0000000-0000-0000-0000-000000000001', 'prod'),
  ('44444444-4444-4444-4444-444444444444', 'shop-staging', 'free', 'd0000000-0000-0000-0000-000000000001', 'staging');

-- domain-scoped key (plaintext test-key-domain, dev-only): may switch among demo-domain tenants.
INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES
  ('33333333-3333-3333-3333-333333333333', 'demo-domain-key', '$2a$10$iH/10gDc7HRk4BoUiaJj3uJ/EDPisrDgkl8Rns250Lm7/5t69DL76', 'domain');
