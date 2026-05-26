package query

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// logsLister is the interface seam that LogsHandler depends on.
// *LogsRepo satisfies this; tests inject a fake.
type logsLister interface {
	List(ctx context.Context, p LogsListParams) ([]LogItem, bool, error)
}

// LogsHandler serves /api/v1/logs.
type LogsHandler struct{ repo logsLister }

// NewLogsHandler constructs a LogsHandler backed by repo (typically *LogsRepo).
func NewLogsHandler(repo logsLister) *LogsHandler { return &LogsHandler{repo: repo} }

// List handles GET /v1/logs with the following query parameters:
//
//	service       repeated string  — rows whose service is in the set
//	severity      repeated string  — rows whose severity_text is in the set
//	ts_from       RFC3339          — start of time window (default: now-1h)
//	ts_to         RFC3339          — end of time window   (default: now+1s)
//	trace_id      hex(32)          — exact trace_id match
//	span_id       hex(16)          — exact span_id match
//	body_contains string           — positionUTF8(body, ?) > 0
//	limit         int              — default 50, max 500
//	offset        int              — default 0
func (h *LogsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	tsFrom, err := parseTsOrDefault(q.Get("ts_from"), time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		http.Error(w, "bad ts_from: must be RFC3339", http.StatusBadRequest)
		return
	}
	tsTo, err := parseTsOrDefault(q.Get("ts_to"), time.Now().UTC().Add(1*time.Second))
	if err != nil {
		http.Error(w, "bad ts_to: must be RFC3339", http.StatusBadRequest)
		return
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			http.Error(w, "limit must be an integer in [1,500]", http.StatusBadRequest)
			return
		}
		limit = n
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, "offset must be a non-negative integer", http.StatusBadRequest)
			return
		}
		offset = n
	}

	if tid := q.Get("trace_id"); tid != "" && !isHex(tid, 32) {
		http.Error(w, "trace_id must be 32 hex chars", http.StatusBadRequest)
		return
	}
	if sid := q.Get("span_id"); sid != "" && !isHex(sid, 16) {
		http.Error(w, "span_id must be 16 hex chars", http.StatusBadRequest)
		return
	}

	items, hasMore, err := h.repo.List(r.Context(), LogsListParams{
		Service:      q["service"],
		Severity:     q["severity"],
		TsFrom:       tsFrom,
		TsTo:         tsTo,
		TraceID:      q.Get("trace_id"),
		SpanID:       q.Get("span_id"),
		BodyContains: q.Get("body_contains"),
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LogsListResponse{Items: items, HasMore: hasMore})
}

// parseTsOrDefault parses s as RFC3339; returns def when s is empty.
func parseTsOrDefault(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// isHex returns true iff s is exactly want lowercase/uppercase hex characters.
func isHex(s string, want int) bool {
	if len(s) != want {
		return false
	}
	for _, c := range strings.ToLower(s) {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
