package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/apikey"
	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/tenant"
)

// fakeResolver implements auth.Resolver in-memory for tests.
type fakeResolver struct {
	keys    map[string]apikey.ApiKey // hashed -> key (we'll iterate; bcrypt has no reverse)
	tenants map[uuid.UUID]tenant.Tenant
}

func (f *fakeResolver) ResolveBearer(ctx context.Context, plain string) (apikey.ApiKey, tenant.Tenant, error) {
	for h, k := range f.keys {
		if apikey.Verify(plain, h) && k.IsActive() {
			return k, f.tenants[k.TenantID], nil
		}
	}
	return apikey.ApiKey{}, tenant.Tenant{}, auth.ErrUnauthorized
}

// TenantByID makes fakeResolver satisfy auth.TenantLookup.
func (f *fakeResolver) TenantByID(ctx context.Context, id uuid.UUID) (tenant.Tenant, error) {
	tn, ok := f.tenants[id]
	if !ok {
		return tenant.Tenant{}, auth.ErrTenantNotFound
	}
	return tn, nil
}

func newFakeResolver(t *testing.T) (*fakeResolver, uuid.UUID, string) {
	t.Helper()
	tID := uuid.New()
	plain := "plain-test-key"
	hashed, err := apikey.Hash(plain)
	require.NoError(t, err)
	f := &fakeResolver{
		keys: map[string]apikey.ApiKey{
			hashed: {TenantID: tID, Name: "p", HashedKey: hashed, Scope: "rw"},
		},
		tenants: map[uuid.UUID]tenant.Tenant{
			tID: {ID: tID, Name: "acme"},
		},
	}
	return f, tID, plain
}

func TestMiddleware_ValidKey_InjectsTenant(t *testing.T) {
	f, wantTenant, plain := newFakeResolver(t)

	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := auth.TenantID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, wantTenant, got)
		assert.Equal(t, "acme", auth.TenantName(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_MissingHeader_401(t *testing.T) {
	f, _, _ := newFakeResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_WrongKey_401(t *testing.T) {
	f, _, _ := newFakeResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_MalformedHeader_401(t *testing.T) {
	f, _, plain := newFakeResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", plain) // missing "Bearer " prefix
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- PLATFORM-ASK-1: service:ai scope + X-Tenant-Id header trust ---

// newServiceAIResolver builds a resolver with one service:ai key (its own
// "ai" tenant) plus a separate target tenant the AI may act on behalf of.
func newServiceAIResolver(t *testing.T) (f *fakeResolver, aiTenant, targetTenant uuid.UUID, plain string) {
	t.Helper()
	aiTenant = uuid.New()
	targetTenant = uuid.New()
	plain = "plain-ai-key"
	hashed, err := apikey.Hash(plain)
	require.NoError(t, err)
	f = &fakeResolver{
		keys: map[string]apikey.ApiKey{
			hashed: {TenantID: aiTenant, Name: "ai", HashedKey: hashed, Scope: auth.ScopeServiceAI},
		},
		tenants: map[uuid.UUID]tenant.Tenant{
			aiTenant:     {ID: aiTenant, Name: "ai-svc"},
			targetTenant: {ID: targetTenant, Name: "acme"},
		},
	}
	return f, aiTenant, targetTenant, plain
}

func TestMiddleware_ServiceAI_HonorsTenantHeader(t *testing.T) {
	f, _, target, plain := newServiceAIResolver(t)

	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := auth.TenantID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, target, got, "service:ai key must adopt the X-Tenant-Id target")
		assert.Equal(t, "acme", auth.TenantName(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", target.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_ServiceAI_NoHeader_PinsToKeyTenant(t *testing.T) {
	f, aiTenant, _, plain := newServiceAIResolver(t)

	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := auth.TenantID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, aiTenant, got, "no header => pin to the key's own tenant")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_NonServiceAI_IgnoresTenantHeader(t *testing.T) {
	// A normal read-write key sets X-Tenant-Id to another tenant; it must be ignored.
	f, keyTenant, plain := newFakeResolver(t) // scope "rw"
	otherTenant := uuid.New()
	f.tenants[otherTenant] = tenant.Tenant{ID: otherTenant, Name: "evil"}

	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := auth.TenantID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, keyTenant, got, "non-service:ai key must stay pinned to its own tenant")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", otherTenant.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_ServiceAI_MalformedTenantHeader_400(t *testing.T) {
	f, _, _, plain := newServiceAIResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called on malformed X-Tenant-Id")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", "not-a-uuid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMiddleware_ServiceAI_UnknownTenant_404(t *testing.T) {
	f, _, _, plain := newServiceAIResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called for unknown tenant")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", uuid.New().String()) // valid uuid, not in store
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
