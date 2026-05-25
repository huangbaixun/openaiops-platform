package chquery

import (
	"context"
	"errors"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// Batch wraps clickhouse-go driver.Batch and pins the tenant captured at
// PrepareBatch time. Append's first arg must equal that tenant_id, and the
// CH Row Policy re-checks at Send.
type Batch struct {
	b   driver.Batch
	tid string
}

// PrepareBatch validates ctx has a tenant_id, query is INSERT-shape, and
// injects the custom_tenant_id session setting so the Row Policy fires on Send.
// Panics on missing tenant (programmer error — auth middleware should have set it).
func (cn *Conn) PrepareBatch(ctx context.Context, query string) (*Batch, error) {
	if !insertShape.MatchString(query) {
		return nil, fmt.Errorf("chquery: PrepareBatch query must have '(tenant_id,' as first column: %q", query)
	}
	tid, err := auth.TenantID(ctx)
	if err != nil {
		panic(fmt.Errorf("chquery: ctx has no tenant_id (auth middleware did not run?): %w", err))
	}
	if cn == nil || cn.c == nil {
		return nil, errors.New("chquery: nil Conn")
	}
	ctxWithSettings := clickhouse.Context(ctx, clickhouse.WithSettings(tenantSettings(tid)))
	b, err := cn.c.PrepareBatch(ctxWithSettings, query)
	if err != nil {
		return nil, fmt.Errorf("chquery: prepare batch: %w", err)
	}
	return &Batch{b: b, tid: tid.String()}, nil
}

// Append validates args[0] is the pinned tenant_id, then delegates.
func (b *Batch) Append(args ...any) error {
	if len(args) == 0 {
		return errors.New("chquery: batch Append needs at least tenant_id as first arg")
	}
	s, ok := args[0].(string)
	if !ok || s != b.tid {
		return fmt.Errorf("chquery: batch Append first arg must be tenant_id %q, got %T %v",
			b.tid, args[0], args[0])
	}
	return b.b.Append(args...)
}

func (b *Batch) Send() error  { return b.b.Send() }
func (b *Batch) Abort() error { return b.b.Abort() }
func (b *Batch) IsSent() bool { return b.b.IsSent() }
func (b *Batch) Rows() int    { return b.b.Rows() }
