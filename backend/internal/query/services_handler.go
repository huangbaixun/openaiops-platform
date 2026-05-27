package query

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// servicesRepoIface is the interface seam ServicesHandler depends on.
// *ServicesRepo satisfies this; tests inject a fake.
type servicesRepoIface interface {
	List(ctx context.Context, window string, limit int, sort string) ([]ServicesListItem, error)
	Detail(ctx context.Context, name, window string) (*ServiceDetailResponse, error)
}

// ServicesHandler serves /api/v1/services and /api/v1/services/{name}.
type ServicesHandler struct{ repo servicesRepoIface }

// NewServicesHandler constructs a ServicesHandler backed by repo (typically *ServicesRepo).
func NewServicesHandler(repo servicesRepoIface) *ServicesHandler {
	return &ServicesHandler{repo: repo}
}

// List handles GET /v1/services with query parameters:
//
//	window  string  one of {15m, 1h, 6h, 24h} (default 1h)
//	limit   int     1..500 (default 100)
//	sort    string  one of {calls, errors, p95} (default calls)
func (h *ServicesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	window := q.Get("window")
	if window == "" {
		window = "1h"
	}
	if WindowToMinutes(window) < 0 {
		http.Error(w, "window must be one of 15m,1h,6h,24h", http.StatusBadRequest)
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

	sort := q.Get("sort")
	if sort == "" {
		sort = "calls"
	}
	switch sort {
	case "calls", "errors", "p95":
	default:
		http.Error(w, "sort must be one of calls,errors,p95", http.StatusBadRequest)
		return
	}

	items, err := h.repo.List(r.Context(), window, limit, sort)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []ServicesListItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ServicesListResponse{Window: window, Items: items})
}

// Detail handles GET /v1/services/{name}.
//
//	window  string  one of {15m, 1h, 6h, 24h} (default 1h)
//
// Returns 404 when the named service has no rows under the calling tenant
// (mirrors traces detail: missing means missing for THIS tenant — never leak
// the existence of another tenant's service).
func (h *ServicesHandler) Detail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}
	if WindowToMinutes(window) < 0 {
		http.Error(w, "window must be one of 15m,1h,6h,24h", http.StatusBadRequest)
		return
	}

	resp, err := h.repo.Detail(r.Context(), name, window)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if resp == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
