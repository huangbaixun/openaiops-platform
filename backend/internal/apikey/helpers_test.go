package apikey_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
)

func newRepo(t *testing.T) (*apikey.Repo, func()) {
	t.Helper()
	db, err := sql.Open("pgx", testDSN)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(),
		"TRUNCATE TABLE api_keys, tenants, metering_events RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	return apikey.NewRepo(db), func() { db.Close() }
}

func mustSeedTenant(t *testing.T, repo *apikey.Repo, name string) uuid.UUID {
	t.Helper()
	db := repo.DB()
	var id uuid.UUID
	err := db.QueryRowContext(context.Background(),
		"INSERT INTO tenants(name) VALUES($1) RETURNING id", name).Scan(&id)
	require.NoError(t, err)
	return id
}
