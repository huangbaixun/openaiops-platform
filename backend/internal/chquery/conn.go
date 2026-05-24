package chquery

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// Conn wraps a clickhouse-go driver connection and enforces MustTenantScope
// on every query. Direct access to the underlying driver.Conn is intentionally
// not exposed — callers must go through Query/Exec to maintain tenant safety.
type Conn struct {
	c driver.Conn
}

// Connect opens a clickhouse connection from a DSN.
//
//	dsn := "clickhouse://user:pass@host:9000/db"
func Connect(ctx context.Context, dsn string) (*Conn, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("chquery: parse dsn: %w", err)
	}
	c, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("chquery: open: %w", err)
	}
	if err := c.Ping(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("chquery: ping: %w", err)
	}
	return &Conn{c: c}, nil
}

// Close releases the underlying driver connection.
func (cn *Conn) Close() error { return cn.c.Close() }

// Query executes a SELECT (or other read) with tenant scoping enforced.
// The query string MUST contain "tenant_id = ?"; MustTenantScope panics otherwise.
// The tenant_id session setting is also injected, so any CH Row Policy referencing
// getSetting('custom_tenant_id') will fire as a defense-in-depth layer.
func (cn *Conn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	q, scopedArgs := MustTenantScope(ctx, query, args...)
	tid, _ := auth.TenantID(ctx)
	ctxWithSettings := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"custom_tenant_id": tid.String(),
	}))
	return cn.c.Query(ctxWithSettings, q, scopedArgs...)
}

// Exec executes an INSERT (or other write) with tenant scoping enforced.
// The query string MUST contain "(tenant_id," as the first column; MustTenantScope
// panics otherwise.
func (cn *Conn) Exec(ctx context.Context, query string, args ...any) error {
	q, scopedArgs := MustTenantScope(ctx, query, args...)
	tid, _ := auth.TenantID(ctx)
	ctxWithSettings := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"custom_tenant_id": tid.String(),
	}))
	return cn.c.Exec(ctxWithSettings, q, scopedArgs...)
}

// QueryRow executes a query that returns a single row, with tenant scoping enforced.
func (cn *Conn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	q, scopedArgs := MustTenantScope(ctx, query, args...)
	tid, _ := auth.TenantID(ctx)
	ctxWithSettings := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"custom_tenant_id": tid.String(),
	}))
	return cn.c.QueryRow(ctxWithSettings, q, scopedArgs...)
}
