package identity

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

type store interface {
	VisibleTenants(ctx context.Context, tid uuid.UUID, scope string) ([]TenantView, error)
	DomainID(ctx context.Context, tid uuid.UUID) (uuid.UUID, error)
	InsertSwitchAudit(ctx context.Context, fromTenant, toTenant, actorKey uuid.UUID) error
}

type Handler struct{ s store }

func NewHandler(s store) *Handler { return &Handler{s: s} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tid, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	views, err := h.s.VisibleTenants(r.Context(), tid, auth.Scope(r.Context()))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

type switchReq struct {
	TenantID string `json:"tenant_id"`
}

func (h *Handler) Switch(w http.ResponseWriter, r *http.Request) {
	from, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	var req switchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TenantID == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	to, err := uuid.Parse(req.TenantID)
	if err != nil {
		http.Error(w, "bad tenant_id", http.StatusBadRequest)
		return
	}
	if auth.Scope(r.Context()) != auth.ScopeDomain {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fromDom, err := h.s.DomainID(r.Context(), from)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	toDom, err := h.s.DomainID(r.Context(), to)
	if err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}
	if fromDom == uuid.Nil || toDom != fromDom {
		http.Error(w, "tenant not in your domain", http.StatusForbidden)
		return
	}
	if err := h.s.InsertSwitchAudit(r.Context(), from, to, auth.KeyID(r.Context())); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tenant_id": to.String()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
