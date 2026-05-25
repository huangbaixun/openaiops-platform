package chquery_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

func ctxWithTenant(t *testing.T) context.Context {
	t.Helper()
	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	return auth.WithTenant(context.Background(), tid, "test-tenant")
}
