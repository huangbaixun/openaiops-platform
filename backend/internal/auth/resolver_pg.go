package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/tenant"
)

type PGResolver struct {
	db *sql.DB
}

func NewPGResolver(db *sql.DB) *PGResolver { return &PGResolver{db: db} }

// ResolveBearer iterates active api_keys and bcrypt-verifies the plaintext.
// Slice 0 assumption: N small (<100 active keys). Slice 5+ will add a key-prefix
// hint column on api_keys to narrow the candidate set before bcrypt-verify.
func (p *PGResolver) ResolveBearer(ctx context.Context, plain string) (apikey.ApiKey, tenant.Tenant, error) {
	query := `
		SELECT k.id, k.tenant_id, k.name, k.hashed_key, k.scope, k.revoked_at, k.last_used_at, k.created_at,
		       t.id, t.name, t.plan, t.rate_limit_per_min, t.data_retention_days, t.created_at
		FROM api_keys k JOIN tenants t ON t.id = k.tenant_id
		WHERE k.revoked_at IS NULL
	`
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return apikey.ApiKey{}, tenant.Tenant{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var k apikey.ApiKey
		var t tenant.Tenant
		if err := rows.Scan(
			&k.ID, &k.TenantID, &k.Name, &k.HashedKey, &k.Scope,
			&k.RevokedAt, &k.LastUsedAt, &k.CreatedAt,
			&t.ID, &t.Name, &t.Plan, &t.RateLimitPerMin, &t.DataRetentionDays, &t.CreatedAt,
		); err != nil {
			return apikey.ApiKey{}, tenant.Tenant{}, err
		}
		if apikey.Verify(plain, k.HashedKey) {
			return k, t, nil
		}
	}
	return apikey.ApiKey{}, tenant.Tenant{}, errors.Join(ErrUnauthorized, sql.ErrNoRows)
}

// TenantByID resolves a tenant by id, satisfying auth.TenantLookup so the
// middleware can honor X-Tenant-Id for service:ai keys (PLATFORM-ASK-1).
// Returns ErrTenantNotFound when the id is unknown.
func (p *PGResolver) TenantByID(ctx context.Context, id uuid.UUID) (tenant.Tenant, error) {
	query := `
		SELECT id, name, plan, rate_limit_per_min, data_retention_days, created_at
		FROM tenants
		WHERE id = $1
	`
	var t tenant.Tenant
	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.Name, &t.Plan, &t.RateLimitPerMin, &t.DataRetentionDays, &t.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return tenant.Tenant{}, ErrTenantNotFound
	}
	if err != nil {
		return tenant.Tenant{}, err
	}
	return t, nil
}
