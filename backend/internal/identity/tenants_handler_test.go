package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

type fakeStore struct {
	views   []TenantView
	domains map[uuid.UUID]uuid.UUID
	audits  int
}

func (f *fakeStore) VisibleTenants(_ context.Context, _ uuid.UUID, _ string) ([]TenantView, error) {
	return f.views, nil
}
func (f *fakeStore) DomainID(_ context.Context, tid uuid.UUID) (uuid.UUID, error) {
	return f.domains[tid], nil
}
func (f *fakeStore) InsertSwitchAudit(_ context.Context, _, _, _ uuid.UUID) error {
	f.audits++
	return nil
}

func ctxWith(tid uuid.UUID, scope string) context.Context {
	ctx := auth.WithTenant(context.Background(), tid, "t")
	ctx = auth.WithScope(ctx, scope)
	ctx = auth.WithKeyID(ctx, uuid.New())
	return ctx
}

func TestList_ReturnsViews(t *testing.T) {
	home := uuid.New()
	fs := &fakeStore{views: []TenantView{{ID: home.String(), Name: "shop-prod", Environment: "prod"}}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil).WithContext(ctxWith(home, "domain"))
	h.List(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got []TenantView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "shop-prod", got[0].Name)
}

func TestSwitch_InDomain_200_Audits(t *testing.T) {
	home, peer, dom := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: dom, peer: dom}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + peer.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "domain"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, fs.audits)
}

func TestSwitch_OutOfDomain_403_NoAudit(t *testing.T) {
	home, other, dom, otherDom := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: dom, other: otherDom}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + other.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "domain"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, 0, fs.audits)
}

func TestSwitch_NonDomainScope_403(t *testing.T) {
	home, peer := uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: uuid.New(), peer: uuid.New()}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + peer.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "read-write"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
