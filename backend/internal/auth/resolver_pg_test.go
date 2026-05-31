//go:build integration

package auth_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

var pgDSN string

func TestMain(m *testing.M) {
	pool, _ := dockertest.NewPool("")
	resource, _ := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres", Tag: "16-alpine",
		Env: []string{"POSTGRES_PASSWORD=test", "POSTGRES_DB=test"},
	}, func(c *docker.HostConfig) { c.AutoRemove = true })
	pgDSN = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable",
		resource.GetPort("5432/tcp"))
	if err := pool.Retry(func() error {
		db, _ := sql.Open("pgx", pgDSN)
		return db.Ping()
	}); err != nil {
		log.Fatal(err)
	}
	db, _ := sql.Open("pgx", pgDSN)
	_ = goose.SetDialect("postgres")
	_ = goose.Up(db, "../../migrations")
	db.Close()
	code := m.Run()
	pool.Purge(resource)
	os.Exit(code)
}

func TestPGResolver_TwoTenants_NoCrossTalk(t *testing.T) {
	ctx := context.Background()
	db, _ := sql.Open("pgx", pgDSN)
	defer db.Close()
	_, err := db.ExecContext(ctx, "TRUNCATE domains, api_keys, tenants RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	var t1ID, t2ID string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name) VALUES('acme') RETURNING id").Scan(&t1ID))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name) VALUES('beta') RETURNING id").Scan(&t2ID))

	h1, _ := apikey.Hash("key-acme")
	h2, _ := apikey.Hash("key-beta")
	_, err = db.ExecContext(ctx, "INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES($1,'k',$2,'rw')", t1ID, h1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES($1,'k',$2,'rw')", t2ID, h2)
	require.NoError(t, err)

	r := auth.NewPGResolver(db)

	_, gotAcme, err := r.ResolveBearer(ctx, "key-acme")
	require.NoError(t, err)
	assert.Equal(t, "acme", gotAcme.Name)

	_, gotBeta, err := r.ResolveBearer(ctx, "key-beta")
	require.NoError(t, err)
	assert.Equal(t, "beta", gotBeta.Name)

	_, _, err = r.ResolveBearer(ctx, "nope")
	assert.Error(t, err)
}

// PLATFORM-ASK-1: PGResolver.TenantByID powers the service:ai X-Tenant-Id path.
func TestPGResolver_TenantByID(t *testing.T) {
	ctx := context.Background()
	db, _ := sql.Open("pgx", pgDSN)
	defer db.Close()
	_, err := db.ExecContext(ctx, "TRUNCATE domains, api_keys, tenants RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	var tID string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name) VALUES('acme') RETURNING id").Scan(&tID))

	r := auth.NewPGResolver(db)

	id, err := uuid.Parse(tID)
	require.NoError(t, err)

	got, err := r.TenantByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "acme", got.Name)
	assert.Equal(t, id, got.ID)

	var domID string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO domains(name) VALUES('acme-corp') RETURNING id").Scan(&domID))
	var dtID string
	require.NoError(t, db.QueryRowContext(ctx,
		"INSERT INTO tenants(name, domain_id, environment) VALUES('shop-prod',$1,'prod') RETURNING id", domID).Scan(&dtID))

	did, err := uuid.Parse(dtID)
	require.NoError(t, err)
	got2, err := r.TenantByID(ctx, did)
	require.NoError(t, err)
	assert.Equal(t, "shop-prod", got2.Name)
	assert.Equal(t, "prod", got2.Environment)
	assert.Equal(t, domID, got2.DomainID.String())

	_, err = r.TenantByID(ctx, uuid.New()) // valid uuid, absent row
	assert.ErrorIs(t, err, auth.ErrTenantNotFound)
}
