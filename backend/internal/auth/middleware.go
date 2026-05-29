package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/tenant"
)

var ErrUnauthorized = errors.New("unauthorized")

// ErrTenantNotFound is returned by TenantLookup when the requested tenant id
// does not exist.
var ErrTenantNotFound = errors.New("tenant not found")

// ScopeServiceAI is the api_keys.scope value that grants cross-tenant access:
// a key with this scope may act on behalf of any tenant by sending the
// X-Tenant-Id header. All other scopes are pinned to the key's own tenant.
// See PLATFORM-ASK-1 / platform spec §3.3.
const ScopeServiceAI = "service:ai"

// HeaderTenantID names the header a service:ai caller uses to select the tenant
// it is acting on behalf of.
const HeaderTenantID = "X-Tenant-Id"

// Resolver resolves a Bearer plaintext key to an api_key + tenant.
// Production = pgx-backed (next task). Tests use in-memory fake.
type Resolver interface {
	ResolveBearer(ctx context.Context, plain string) (apikey.ApiKey, tenant.Tenant, error)
}

// TenantLookup is an optional capability a Resolver may implement to support
// service:ai cross-tenant access: it resolves a tenant by id so the middleware
// can validate an X-Tenant-Id header and adopt the target tenant's identity.
// Returns ErrTenantNotFound when the id is unknown.
type TenantLookup interface {
	TenantByID(ctx context.Context, id uuid.UUID) (tenant.Tenant, error)
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

			// service:ai keys may target any tenant via X-Tenant-Id. Every other
			// scope ignores the header entirely and stays pinned to its own tenant.
			if k.Scope == ScopeServiceAI {
				if raw := req.Header.Get(HeaderTenantID); raw != "" {
					target, err := resolveTargetTenant(req.Context(), r, raw)
					switch {
					case errors.Is(err, errBadTenantID):
						http.Error(w, "invalid X-Tenant-Id", http.StatusBadRequest)
						return
					case errors.Is(err, ErrTenantNotFound):
						http.Error(w, "tenant not found", http.StatusNotFound)
						return
					case err != nil:
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					tn = target
				}
			}

			ctx := WithTenant(req.Context(), tn.ID, tn.Name)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

var errBadTenantID = errors.New("malformed tenant id")

// resolveTargetTenant parses raw as a UUID and resolves it via the Resolver's
// optional TenantLookup capability.
func resolveTargetTenant(ctx context.Context, r Resolver, raw string) (tenant.Tenant, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return tenant.Tenant{}, errBadTenantID
	}
	lookup, ok := r.(TenantLookup)
	if !ok {
		// Resolver can't validate cross-tenant targets; fail closed.
		return tenant.Tenant{}, ErrTenantNotFound
	}
	return lookup.TenantByID(ctx, id)
}
