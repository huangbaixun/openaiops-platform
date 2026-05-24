package chquery_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

var (
	tenantAID = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	tenantBID = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
)

func ctxA() context.Context {
	return auth.WithTenant(context.Background(), tenantAID, "tenant-A")
}
func ctxB() context.Context {
	return auth.WithTenant(context.Background(), tenantBID, "tenant-B")
}

func TestMustTenantScope_PanicsWithoutTenant(t *testing.T) {
	assert.Panics(t, func() {
		chquery.MustTenantScope(context.Background(), "SELECT * FROM x WHERE tenant_id = ?")
	})
}

func TestMustTenantScope_SelectInjectsTenantArg(t *testing.T) {
	q, args := chquery.MustTenantScope(ctxA(),
		"SELECT * FROM traces WHERE tenant_id = ? AND service = ?", "checkout")

	assert.Equal(t, "SELECT * FROM traces WHERE tenant_id = ? AND service = ?", q)
	require.Len(t, args, 2)
	assert.Equal(t, tenantAID.String(), args[0])
	assert.Equal(t, "checkout", args[1])
}

func TestMustTenantScope_SelectRejectsMissingPlaceholder(t *testing.T) {
	assert.Panics(t, func() {
		chquery.MustTenantScope(ctxA(), "SELECT * FROM traces WHERE service = ?", "checkout")
	})
}

func TestMustTenantScope_InsertInjectsTenantArg(t *testing.T) {
	q, args := chquery.MustTenantScope(ctxB(),
		"INSERT INTO traces (tenant_id, trace_id, service) VALUES (?, ?, ?)",
		"trace-xyz", "checkout")

	assert.Equal(t, "INSERT INTO traces (tenant_id, trace_id, service) VALUES (?, ?, ?)", q)
	require.Len(t, args, 3)
	assert.Equal(t, tenantBID.String(), args[0])
	assert.Equal(t, "trace-xyz", args[1])
	assert.Equal(t, "checkout", args[2])
}

func TestMustTenantScope_InsertRejectsMissingTenantColumn(t *testing.T) {
	assert.Panics(t, func() {
		chquery.MustTenantScope(ctxB(), "INSERT INTO traces (trace_id, service) VALUES (?, ?)",
			"trace-xyz", "checkout")
	})
}

func TestMustTenantScope_AcceptsWhitespaceVariations(t *testing.T) {
	cases := []string{
		"SELECT * FROM x WHERE tenant_id=?",
		"SELECT * FROM x WHERE tenant_id  =  ?",
		"SELECT * FROM x WHERE TENANT_ID = ?",
		"select * from x where tenant_id = ?",
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_, _ = chquery.MustTenantScope(ctxA(), q)
			})
		})
	}
}
