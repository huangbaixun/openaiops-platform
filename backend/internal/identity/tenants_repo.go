package identity

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

type TenantView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// VisibleTenants returns the tenants the caller may enumerate. A domain-scoped caller
// whose tenant has a non-NULL domain_id sees every tenant in that domain; otherwise just
// the caller's own tenant.
func (r *Repo) VisibleTenants(ctx context.Context, tid uuid.UUID, scope string) ([]TenantView, error) {
	if scope == "domain" {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, name, COALESCE(environment, '')
			FROM tenants
			WHERE domain_id IS NOT NULL
			  AND domain_id = (SELECT domain_id FROM tenants WHERE id = $1)
			ORDER BY environment, name
		`, tid)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TenantView
		for rows.Next() {
			var v TenantView
			if err := rows.Scan(&v.ID, &v.Name, &v.Environment); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(out) > 0 {
			return out, nil
		}
		// domain key whose own tenant has NULL domain_id → fall through to single.
	}
	return r.singleTenant(ctx, tid)
}

func (r *Repo) singleTenant(ctx context.Context, tid uuid.UUID) ([]TenantView, error) {
	var v TenantView
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(environment, '') FROM tenants WHERE id = $1`, tid,
	).Scan(&v.ID, &v.Name, &v.Environment)
	if err != nil {
		return nil, err
	}
	return []TenantView{v}, nil
}

// DomainID returns a tenant's domain_id (uuid.Nil if NULL/absent row error).
func (r *Repo) DomainID(ctx context.Context, tid uuid.UUID) (uuid.UUID, error) {
	var dn uuid.NullUUID
	err := r.db.QueryRowContext(ctx, `SELECT domain_id FROM tenants WHERE id = $1`, tid).Scan(&dn)
	if err != nil {
		return uuid.Nil, err
	}
	if dn.Valid {
		return dn.UUID, nil
	}
	return uuid.Nil, nil
}

// InsertSwitchAudit records one tenant-switch action.
func (r *Repo) InsertSwitchAudit(ctx context.Context, fromTenant, toTenant, actorKey uuid.UUID) error {
	var actor any
	if actorKey != uuid.Nil {
		actor = actorKey
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_log (tenant_id, actor_key_id, action, from_tenant_id, to_tenant_id)
		VALUES ($1, $2, 'tenant_switch', $3, $4)
	`, fromTenant, actor, fromTenant, toTenant)
	return err
}
