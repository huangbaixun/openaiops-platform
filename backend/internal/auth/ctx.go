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
