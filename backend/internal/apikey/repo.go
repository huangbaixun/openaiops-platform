package apikey

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("api key not found")

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// DB exposed for test helpers; production code should not call this.
func (r *Repo) DB() *sql.DB { return r.db }

func (r *Repo) Insert(ctx context.Context, k ApiKey) (ApiKey, error) {
	query := `
		INSERT INTO api_keys(tenant_id, name, hashed_key, scope)
		VALUES($1, $2, $3, $4)
		RETURNING id, created_at
	`
	var id uuid.UUID
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, query,
		k.TenantID, k.Name, k.HashedKey, k.Scope,
	).Scan(&id, &createdAt)
	if err != nil {
		return ApiKey{}, err
	}
	k.ID = id
	k.CreatedAt = createdAt
	return k, nil
}

func (r *Repo) FindByHashedKey(ctx context.Context, hashed string) (ApiKey, error) {
	query := `
		SELECT id, tenant_id, name, hashed_key, scope, revoked_at, last_used_at, created_at
		FROM api_keys
		WHERE hashed_key = $1
	`
	var k ApiKey
	err := r.db.QueryRowContext(ctx, query, hashed).Scan(
		&k.ID, &k.TenantID, &k.Name, &k.HashedKey, &k.Scope,
		&k.RevokedAt, &k.LastUsedAt, &k.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ApiKey{}, ErrNotFound
	}
	return k, err
}

func (r *Repo) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}
