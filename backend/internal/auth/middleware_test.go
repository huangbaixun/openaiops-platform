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
