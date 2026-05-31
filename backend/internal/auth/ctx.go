package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type ctxKey int

const (
	tenantIDKey ctxKey = iota
	tenantNameKey
	scopeKey
	keyIDKey
)

var ErrNoTenant = errors.New("no tenant in context")

func WithTenant(ctx context.Context, id uuid.UUID, name string) context.Context {
	ctx = context.WithValue(ctx, tenantIDKey, id)
	ctx = context.WithValue(ctx, tenantNameKey, name)
	return ctx
}

func TenantID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(tenantIDKey).(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrNoTenant
	}
	return id, nil
}

func TenantName(ctx context.Context) string {
	name, _ := ctx.Value(tenantNameKey).(string)
	return name
}

// WithScope stores the resolved api key's scope so identity handlers (GET /api/v1/tenants)
// can decide what the caller may enumerate.
func WithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, scopeKey, scope)
}

func Scope(ctx context.Context) string {
	s, _ := ctx.Value(scopeKey).(string)
	return s
}

// WithKeyID stores the resolved api key id (the audit actor).
func WithKeyID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keyIDKey, id)
}

func KeyID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(keyIDKey).(uuid.UUID)
	return id
}
