// Package chtest spins up an ephemeral ClickHouse server via dockertest and
// applies CH migrations against it for integration tests. It lives under
// internal/chquery/ so that the no-bare-clickhouse-go lint (lint-no-bare-ch.sh)
// can permit raw clickhouse-go usage here without permitting it in business
// packages. Integration tests in internal/query/, internal/ingest/, etc. should
// call StartCH() instead of importing clickhouse-go directly.
package chtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

// Fixture is a running ephemeral ClickHouse instance with the openaiops DB
// created and the requested migrations applied. Callers should defer Close().
type Fixture struct {
	DSN      string // openaiops DB DSN (clickhouse://...openaiops)
	pool     *dockertest.Pool
	resource *dockertest.Resource
}

// Close tears down the underlying container. Safe to call multiple times.
func (f *Fixture) Close() error {
	if f == nil || f.pool == nil || f.resource == nil {
		return nil
	}
	err := f.pool.Purge(f.resource)
	f.resource = nil
	return err
}

// FatalReporter is the minimal subset of testing.TB that StartCH needs. Both
// *testing.T and a thin TestMain adapter (one that routes Fatalf to log.Fatalf)
// satisfy it — see internal/query/test_helpers_test.go for the TestMain usage.
type FatalReporter interface {
	Helper()
	Fatalf(format string, args ...any)
}

// StartCH starts ClickHouse via dockertest, creates the openaiops DB, and
// applies each migration file from backend/ch-migrations/ given by name
// (e.g. "20260525120000_create_traces_v1.sql"). The custom_settings.xml
// from internal/chquery/testdata/ is bind-mounted so getSetting('custom_*')
// works (Row Policy requirement, see ADR-0001 §3.3).
//
// On any failure StartCH calls t.Fatalf — tests should not need to check err.
func StartCH(t FatalReporter, migrationNames ...string) *Fixture {
	t.Helper()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("chtest: dockertest pool: %s", err)
	}

	_, this, _, _ := runtime.Caller(0)
	// this = .../backend/internal/chquery/chtest/fixture.go
	chqueryDir := filepath.Dir(filepath.Dir(this))      // .../internal/chquery
	backendDir := filepath.Dir(filepath.Dir(chqueryDir)) // .../backend
	customXML := filepath.Join(chqueryDir, "testdata", "custom_settings.xml")
	migrationsDir := filepath.Join(backendDir, "ch-migrations")

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "clickhouse/clickhouse-server",
		Tag:        "23.12-alpine",
		Env: []string{
			"CLICKHOUSE_USER=openaiops",
			"CLICKHOUSE_PASSWORD=openaiops",
			// Required so CREATE ROW POLICY in migrations succeeds.
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1",
		},
	}, func(c *docker.HostConfig) {
		c.AutoRemove = true
		c.Binds = []string{customXML + ":/etc/clickhouse-server/config.d/custom_settings.xml:ro"}
	})
	if err != nil {
		t.Fatalf("chtest: start ch: %s", err)
	}

	port := res.GetPort("9000/tcp")
	defaultDSN := fmt.Sprintf("clickhouse://openaiops:openaiops@localhost:%s/default", port)
	openaiopsDSN := fmt.Sprintf("clickhouse://openaiops:openaiops@localhost:%s/openaiops", port)

	pool.MaxWait = 60 * time.Second
	if err := pool.Retry(func() error {
		opts, perr := clickhousedriver.ParseDSN(defaultDSN)
		if perr != nil {
			return perr
		}
		c, oerr := clickhousedriver.Open(opts)
		if oerr != nil {
			return oerr
		}
		defer c.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return c.Ping(ctx)
	}); err != nil {
		_ = pool.Purge(res)
		t.Fatalf("chtest: ping ch: %s", err)
	}

	// Create openaiops DB.
	{
		opts, _ := clickhousedriver.ParseDSN(defaultDSN)
		raw, _ := clickhousedriver.Open(opts)
		if err := raw.Exec(context.Background(), "CREATE DATABASE IF NOT EXISTS openaiops"); err != nil {
			raw.Close()
			_ = pool.Purge(res)
			t.Fatalf("chtest: create openaiops db: %s", err)
		}
		raw.Close()
	}

	// Apply requested migrations against openaiops.
	{
		opts, _ := clickhousedriver.ParseDSN(openaiopsDSN)
		raw, _ := clickhousedriver.Open(opts)
		for _, name := range migrationNames {
			path := filepath.Join(migrationsDir, name)
			sqlBytes, err := os.ReadFile(path)
			if err != nil {
				raw.Close()
				_ = pool.Purge(res)
				t.Fatalf("chtest: read migration %s: %s", name, err)
			}
			// applyMigration splits the SQL file on ";\n" and runs each statement
			// individually. CONSTRAINT: migration authors MUST use LF line endings
			// (not CRLF) and MUST NOT include a bare ";\n" sequence inside a
			// statement body (e.g., in a multi-line comment or string literal).
			// If a future migration needs that, switch to the CH HTTP batch endpoint
			// or a real SQL tokenizer.
			for _, stmt := range strings.Split(string(sqlBytes), ";\n") {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || allCommentLines(stmt) {
					continue
				}
				if err := raw.Exec(context.Background(), stmt); err != nil {
					raw.Close()
					_ = pool.Purge(res)
					t.Fatalf("chtest: apply %s stmt %q: %s", name, stmt, err)
				}
			}
		}
		raw.Close()
	}

	return &Fixture{DSN: openaiopsDSN, pool: pool, resource: res}
}

// allCommentLines returns true if every non-empty line of s is an SQL
// line comment ("-- ..."). Used to skip migration segments that contain
// only commentary after splitting on ";\n".
func allCommentLines(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, "--") {
			return false
		}
	}
	return true
}
