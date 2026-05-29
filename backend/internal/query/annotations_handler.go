package query

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// annotationsStore is the interface seam AnnotationsHandler depends on.
// *AnnotationsRepo satisfies it; tests inject a fake.
type annotationsStore interface {
	Insert(ctx context.Context, tenantID uuid.UUID, in AnnotationInput, idemKey string) (uuid.UUID, bool, error)
	List(ctx context.Context, tenantID uuid.UUID, targetType, targetID string, limit int) ([]Annotation, error)
}

// AnnotationsHandler serves POST/GET /v1/annotations.
type AnnotationsHandler struct{ store annotationsStore }

func NewAnnotationsHandler(store annotationsStore) *AnnotationsHandler {
	return &AnnotationsHandler{store: store}
}

type createAnnotationReq struct {
	TenantID   string          `json:"tenant_id"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	TS         string          `json:"ts"`
}

func validTargetType(t string) bool { return t == "trace" || t == "service" }

// Create handles POST /v1/annotations.
func (h *AnnotationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}

	var req createAnnotationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Cross-tenant write guard (AC#3): a body tenant_id, if present, must match
	// the effective tenant resolved by auth (ASK-1). We never trust the body.
	if req.TenantID != "" && req.TenantID != tenantID.String() {
		http.Error(w, "tenant_id does not match authenticated tenant", http.StatusForbidden)
		return
	}
	if !validTargetType(req.TargetType) {
		http.Error(w, "target_type must be one of trace,service", http.StatusBadRequest)
		return
	}
	if req.TargetID == "" || req.Kind == "" || len(req.Payload) == 0 || bytes.Equal(bytes.TrimSpace(req.Payload), []byte("null")) {
		http.Error(w, "target_id, kind, payload are required", http.StatusBadRequest)
		return
	}
	ts, err := time.Parse(time.RFC3339, req.TS)
	if err != nil {
		http.Error(w, "ts must be RFC3339", http.StatusBadRequest)
		return
	}

	id, created, err := h.store.Insert(r.Context(), tenantID, AnnotationInput{
		TargetType: req.TargetType, TargetID: req.TargetID, Kind: req.Kind,
		Payload: req.Payload, TS: ts,
	}, r.Header.Get("Idempotency-Key"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"annotation_id": id.String()})
}

// List handles GET /v1/annotations?target_type=...&target_id=...&limit=...
func (h *AnnotationsHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	targetType := q.Get("target_type")
	if !validTargetType(targetType) {
		http.Error(w, "target_type must be one of trace,service", http.StatusBadRequest)
		return
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			http.Error(w, "limit must be an integer in [1,500]", http.StatusBadRequest)
			return
		}
		limit = n
	}
	items, err := h.store.List(r.Context(), tenantID, targetType, q.Get("target_id"), limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}
