-- Seed two tenants + one API key each for Slice 0 smoke tests, plus one
-- service:ai cross-tenant key for PLATFORM-ASK-1.
-- Hashes generated via `go run ./backend/cmd/seed-hash <plaintext>` (bcrypt cost 10).
-- Plaintexts test-key-acme / test-key-beta / test-key-ai are intentionally PUBLIC dev-only.

TRUNCATE api_keys, tenants, metering_events RESTART IDENTITY CASCADE;

INSERT INTO tenants(id, name, plan) VALUES
  ('11111111-1111-1111-1111-111111111111', 'acme', 'free'),
  ('22222222-2222-2222-2222-222222222222', 'beta', 'free'),
  ('99999999-9999-9999-9999-999999999999', 'ai-svc', 'free');

INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES
  ('11111111-1111-1111-1111-111111111111', 'acme-primary', '$2a$10$/1JEzjuI8M5EVtVVVqMFPuZ3E.WZu1tJBCpiRsJf8PFrWw.ZbVCVG', 'read-write'),
  ('22222222-2222-2222-2222-222222222222', 'beta-primary', '$2a$10$dXNGJGtlwM11osqvLPLcceGRNcD5JKJ/FkNdm2TSETTJjJG4wQj.y', 'read-write'),
  -- service:ai key (plaintext test-key-ai, dev-only): may target any tenant via the X-Tenant-Id header (PLATFORM-ASK-1).
  ('99999999-9999-9999-9999-999999999999', 'ai-service', '$2a$10$tzDv/m3Wv9hiej64BT/tfOev1nJxCQS3d8OiOE52taSJg5O37z3ai', 'service:ai');
