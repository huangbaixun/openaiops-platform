package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/tenant"
)

var ErrUnauthorized = errors.New("unauthorized")

// Resolver resolves a Bearer plaintext key to an api_key + tenant.
// Production = pgx-backed (next task). Tests use in-memory fake.
type Resolver interface {
	ResolveBearer(ctx context.Context, plain string) (apikey.ApiKey, tenant.Tenant, error)
}

func Middleware(r Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			h := req.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			plain := strings.TrimPrefix(h, "Bearer ")
			k, tn, err := r.ResolveBearer(req.Context(), plain)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = k // future: stash key id for audit
			ctx := WithTenant(req.Context(), tn.ID, tn.Name)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}
