package apikey_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
)

var testDSN string

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(c *docker.HostConfig) {
		c.AutoRemove = true
	})
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}
	testDSN = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable",
		resource.GetPort("5432/tcp"))

	if err := pool.Retry(func() error {
		db, err := sql.Open("pgx", testDSN)
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		log.Fatalf("could not ping postgres: %s", err)
	}

	db, _ := sql.Open("pgx", testDSN)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.Up(db, "../../migrations"); err != nil {
		log.Fatalf("migrate up: %s", err)
	}
	db.Close()

	code := m.Run()
	pool.Purge(resource)
	os.Exit(code)
}

func TestRepo_InsertAndFindByHashedKey(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newRepo(t)
	defer cleanup()

	tenantID := mustSeedTenant(t, repo, "acme")

	hashed, _ := apikey.Hash("plain-key-123")
	inserted, err := repo.Insert(ctx, apikey.ApiKey{
		TenantID:  tenantID,
		Name:      "primary",
		HashedKey: hashed,
		Scope:     "read-write",
	})
	require.NoError(t, err)
	require.NotEqual(t, "", inserted.ID.String())

	found, err := repo.FindByHashedKey(ctx, hashed)
	require.NoError(t, err)
	assert.Equal(t, inserted.ID, found.ID)
	assert.Equal(t, tenantID, found.TenantID)
	assert.True(t, found.IsActive())
}

func TestRepo_FindByHashedKey_NotFound(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newRepo(t)
	defer cleanup()

	_, err := repo.FindByHashedKey(ctx, "nonexistent-hash")
	assert.ErrorIs(t, err, apikey.ErrNotFound)
}

func TestRepo_RevokedKey_IsNotActive(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newRepo(t)
	defer cleanup()

	tenantID := mustSeedTenant(t, repo, "beta")
	hashed, _ := apikey.Hash("plain-revoked")
	inserted, err := repo.Insert(ctx, apikey.ApiKey{
		TenantID: tenantID, Name: "p", HashedKey: hashed, Scope: "read",
	})
	require.NoError(t, err)

	require.NoError(t, repo.Revoke(ctx, inserted.ID))

	found, err := repo.FindByHashedKey(ctx, hashed)
	require.NoError(t, err)
	assert.False(t, found.IsActive())
}
