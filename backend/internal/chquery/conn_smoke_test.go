package chquery_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

var (
	smokeDSN           string
	clickhouseParseDSN = clickhousedriver.ParseDSN
	clickhouseOpen     = clickhousedriver.Open
)

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}

	// Resolve the testdata config directory relative to this source file so the
	// bind mount works regardless of the working directory when 'go test' runs.
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata")

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "clickhouse/clickhouse-server",
		Tag:        "23.12-alpine",
		Env: []string{
			"CLICKHOUSE_USER=openaiops",
			"CLICKHOUSE_PASSWORD=openaiops",
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1",
		},
	}, func(c *docker.HostConfig) {
		c.AutoRemove = true
		// Mount custom_settings.xml so CH accepts 'custom_' prefixed settings.
		// In production, this config is applied via Helm/Ansible CH configuration.
		c.Binds = []string{
			filepath.Join(testdataDir, "custom_settings.xml") +
				":/etc/clickhouse-server/config.d/custom_settings.xml:ro",
		}
	})
	if err != nil {
		log.Fatalf("could not start clickhouse: %s", err)
	}

	chPort := resource.GetPort("9000/tcp")
	// Use default DB for readiness probe — the openaiops DB is created after CH is up.
	defaultDSN := fmt.Sprintf("clickhouse://openaiops:openaiops@localhost:%s/default", chPort)
	smokeDSN = fmt.Sprintf("clickhouse://openaiops:openaiops@localhost:%s/openaiops", chPort)

	pool.MaxWait = 60 * time.Second

	// Step 1: wait for CH to be reachable on the default DB.
	if err := pool.Retry(func() error {
		opts, parseErr := clickhouseParseDSN(defaultDSN)
		if parseErr != nil {
			return parseErr
		}
		rawConn, openErr := clickhouseOpen(opts)
		if openErr != nil {
			return openErr
		}
		defer rawConn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return rawConn.Ping(ctx)
	}); err != nil {
		log.Fatalf("could not ping clickhouse: %s", err)
	}

	// Step 2: create the openaiops database.
	{
		opts, parseErr := clickhouseParseDSN(defaultDSN)
		if parseErr != nil {
			log.Fatalf("could not parse default DSN: %s", parseErr)
		}
		rawConn, openErr := clickhouseOpen(opts)
		if openErr != nil {
			log.Fatalf("could not open default conn: %s", openErr)
		}
		ctx := context.Background()
		if execErr := rawConn.Exec(ctx, `CREATE DATABASE IF NOT EXISTS openaiops`); execErr != nil {
			log.Fatalf("could not create openaiops db: %s", execErr)
		}
		rawConn.Close()
	}

	// Step 3: verify chquery.Connect works against the openaiops DB.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, connErr := chquery.Connect(ctx, smokeDSN)
		if connErr != nil {
			log.Fatalf("could not connect to openaiops db: %s", connErr)
		}
		c.Close()
	}

	code := m.Run()
	_ = pool.Purge(resource)
	os.Exit(code)
}

// setupSmokeTable opens a raw clickhouse-go connection (bypassing chquery) and
// creates the _chscope_smoke table + row policy. This is the equivalent of the
// production ch-migrate path: DDL is run by trusted infra, not by app code.
// chquery.Conn is intentionally not used here — its API does not (and must not)
// allow DDL without a tenant context.
func setupSmokeTable(t *testing.T) {
	t.Helper()
	opts, err := clickhouseParseDSN(smokeDSN)
	require.NoError(t, err)
	rawConn, err := clickhouseOpen(opts)
	require.NoError(t, err)
	defer rawConn.Close()

	ctx := context.Background()
	stmts := []string{
		`DROP TABLE IF EXISTS _chscope_smoke`,
		`DROP ROW POLICY IF EXISTS smoke_isolation ON _chscope_smoke`,
		`CREATE TABLE _chscope_smoke (
			tenant_id LowCardinality(String),
			id UInt32,
			payload String
		) ENGINE = MergeTree ORDER BY (tenant_id, id)`,
		`CREATE ROW POLICY smoke_isolation ON _chscope_smoke
			USING tenant_id = getSetting('custom_tenant_id') TO openaiops`,
	}
	for _, s := range stmts {
		require.NoError(t, rawConn.Exec(ctx, s), "stmt: %s", s)
	}
}

func TestSmoke_TenantIsolation(t *testing.T) {
	conn, err := chquery.Connect(context.Background(), smokeDSN)
	require.NoError(t, err)
	defer conn.Close()

	setupSmokeTable(t)

	tenantAID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantBID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctxA := auth.WithTenant(context.Background(), tenantAID, "tenant-A")
	ctxB := auth.WithTenant(context.Background(), tenantBID, "tenant-B")

	// tenant A writes 3 rows
	for i := 1; i <= 3; i++ {
		require.NoError(t, conn.Exec(ctxA,
			`INSERT INTO _chscope_smoke (tenant_id, id, payload) VALUES (?, ?, ?)`,
			uint32(i), fmt.Sprintf("A-%d", i)))
	}
	// tenant B writes 2 rows
	for i := 1; i <= 2; i++ {
		require.NoError(t, conn.Exec(ctxB,
			`INSERT INTO _chscope_smoke (tenant_id, id, payload) VALUES (?, ?, ?)`,
			uint32(i), fmt.Sprintf("B-%d", i)))
	}

	// tenant A queries → must see only A's 3 rows
	var countA uint64
	require.NoError(t, conn.QueryRow(ctxA,
		`SELECT count() FROM _chscope_smoke WHERE tenant_id = ?`).Scan(&countA))
	assert.Equal(t, uint64(3), countA, "tenant A should see exactly 3 rows")

	// tenant B queries → must see only B's 2 rows
	var countB uint64
	require.NoError(t, conn.QueryRow(ctxB,
		`SELECT count() FROM _chscope_smoke WHERE tenant_id = ?`).Scan(&countB))
	assert.Equal(t, uint64(2), countB, "tenant B should see exactly 2 rows")
}

func TestSmoke_BatchTenantIsolation(t *testing.T) {
	conn, err := chquery.Connect(context.Background(), smokeDSN)
	require.NoError(t, err)
	defer conn.Close()
	setupSmokeTable(t)

	ctxA := auth.WithTenant(context.Background(),
		uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "tenant-A")

	batch, err := conn.PrepareBatch(ctxA,
		`INSERT INTO _chscope_smoke (tenant_id, id, payload) VALUES`)
	require.NoError(t, err)

	require.NoError(t, batch.Append("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", uint32(1), "batch-A-1"))
	require.NoError(t, batch.Append("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", uint32(2), "batch-A-2"))

	err = batch.Append("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", uint32(3), "should-not-land")
	require.Error(t, err)

	require.NoError(t, batch.Send())

	var n uint64
	require.NoError(t, conn.QueryRow(ctxA,
		`SELECT count() FROM _chscope_smoke WHERE tenant_id = ?`).Scan(&n))
	assert.Equal(t, uint64(2), n)
}

func TestSmoke_PanicWithoutTenant(t *testing.T) {
	conn, err := chquery.Connect(context.Background(), smokeDSN)
	require.NoError(t, err)
	defer conn.Close()

	setupSmokeTable(t)

	assert.Panics(t, func() {
		_, _ = conn.Query(context.Background(),
			`SELECT count() FROM _chscope_smoke WHERE tenant_id = ?`)
	})
}
