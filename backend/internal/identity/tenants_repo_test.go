//go:build integration

package identity_test

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

	"github.com/huangbaixun/openaiops-platform/backend/internal/identity"
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

func TestRepo_VisibleTenants_DomainPeersVsSingle(t *testing.T) {
	ctx := context.Background()
	db, _ := sql.Open("pgx", pgDSN)
	defer db.Close()
	_, _ = db.ExecContext(ctx, "TRUNCATE audit_log, api_keys, tenants, domains RESTART IDENTITY CASCADE")

	var dom string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO domains(name) VALUES('d') RETURNING id").Scan(&dom))
	var prod, stg, lone string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name,domain_id,environment) VALUES('prod',$1,'prod') RETURNING id", dom).Scan(&prod))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name,domain_id,environment) VALUES('stg',$1,'staging') RETURNING id", dom).Scan(&stg))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name) VALUES('lone') RETURNING id").Scan(&lone))

	repo := identity.NewRepo(db)
	prodID := uuid.MustParse(prod)
	peers, err := repo.VisibleTenants(ctx, prodID, "domain")
	require.NoError(t, err)
	assert.Len(t, peers, 2)

	loneID := uuid.MustParse(lone)
	single, err := repo.VisibleTenants(ctx, loneID, "read-write")
	require.NoError(t, err)
	assert.Len(t, single, 1)

	require.NoError(t, repo.InsertSwitchAudit(ctx, prodID, uuid.MustParse(stg), uuid.New()))
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_log WHERE action='tenant_switch'").Scan(&n))
	assert.Equal(t, 1, n)
}
