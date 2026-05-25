package query

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// TracesHandler serves /api/v1/traces* endpoints. T8 fills List; T9 fills Detail.
type TracesHandler struct{ repo *TracesRepo }

func NewTracesHandler(ch *chquery.Conn) *TracesHandler {
	return &TracesHandler{repo: NewTracesRepo(ch)}
}

var errBadParams = errors.New("bad params")

// parseListParams decodes /api/v1/traces query string into a validated ListParams.
// Sort/Order are whitelisted here (not in the repo) so the handler returns 400 — not 500 —
// on garbage input; the repo's whitelist remains as a defense-in-depth backstop.
func parseListParams(r *http.Request) (ListParams, error) {
	q := r.URL.Query()
	now := time.Now().UTC()
	p := ListParams{
		TsFrom: now.Add(-1 * time.Hour),
		TsTo:   now,
		Limit:  100,
		Offset: 0,
		Sort:   "ts",
		Order:  "desc",
	}
	if v := q.Get("ts_from"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return p, err
		}
		p.TsFrom = t
	}
	if v := q.Get("ts_to"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return p, err
		}
		p.TsTo = t
	}
	p.Service = q.Get("service")
	p.Operation = q.Get("operation")
	if v := q.Get("min_duration_ms"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return p, err
		}
		if f < 0 {
			f = 0
		}
		p.MinDurationMs = f
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return p, err
		}
		if n < 1 {
			n = 1
		}
		if n > 1000 {
			n = 1000
		}
		p.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return p, errBadParams
		}
		p.Offset = n
	}
	if v := q.Get("sort"); v != "" {
		if v != "ts" && v != "duration" {
			return p, errBadParams
		}
		p.Sort = v
	}
	if v := q.Get("order"); v != "" {
		if v != "asc" && v != "desc" {
			return p, errBadParams
		}
		p.Order = v
	}
	return p, nil
}

func (h *TracesHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := parseListParams(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	items, hasMore, err := h.repo.List(r.Context(), p)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TraceListResponse{Items: items, HasMore: hasMore})
}

func (h *TracesHandler) Detail(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "trace_id")
	if traceID == "" {
		http.Error(w, "missing trace_id", http.StatusBadRequest)
		return
	}
	spans, err := h.repo.Detail(r.Context(), traceID)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if len(spans) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TraceDetailResponse{TraceID: traceID, Spans: spans})
}
