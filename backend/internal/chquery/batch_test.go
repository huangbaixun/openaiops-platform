package chquery_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

func TestPrepareBatch_PanicsWithoutTenant(t *testing.T) {
	var cn *chquery.Conn
	assert.Panics(t, func() {
		_, _ = cn.PrepareBatch(context.Background(), "INSERT INTO foo (tenant_id, x) VALUES")
	})
}

func TestPrepareBatch_RejectsNonInsertShape(t *testing.T) {
	ctx := ctxWithTenant(t)
	var cn *chquery.Conn
	_, err := cn.PrepareBatch(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("expected shape error, got nil")
	}
}
