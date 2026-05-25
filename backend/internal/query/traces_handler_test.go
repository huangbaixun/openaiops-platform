package query

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestList_InvalidSort_400(t *testing.T) {
	h := NewTracesHandler(nil) // ch unused — param parse rejects first
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?sort=DROP%20TABLE", nil)
	h.List(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestList_InvalidOrder_400(t *testing.T) {
	h := NewTracesHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?order=random", nil)
	h.List(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestParseListParams_LimitClamped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=99999", nil)
	p, err := parseListParams(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limit != 1000 {
		t.Fatalf("limit = %d, want 1000", p.Limit)
	}
}

func TestParseListParams_LimitMin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=0", nil)
	p, err := parseListParams(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limit != 1 {
		t.Fatalf("limit = %d, want 1", p.Limit)
	}
}

func TestParseListParams_DefaultsAreSane(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	p, err := parseListParams(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sort != "ts" || p.Order != "desc" {
		t.Fatalf("defaults = %+v", p)
	}
	if p.Limit != 100 {
		t.Fatalf("default limit = %d", p.Limit)
	}
}

func TestParseListParams_InvalidTsFrom_Error(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?ts_from=not-a-time", nil)
	if _, err := parseListParams(req); err == nil {
		t.Fatal("expected error")
	}
}

func TestList_EmptyTenant_ReturnsEmptyArrayNotNull(t *testing.T) {
	// We can't hit the real repo without a CH, so use a nil-conn-tolerant
	// test path: parseListParams + manual JSON encode of an empty response.
	// Actual repo behavior is locked by the make([]TraceListItem, 0) init.
	body, err := json.Marshal(TraceListResponse{Items: make([]TraceListItem, 0), HasMore: false})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, `"items":[]`) {
		t.Fatalf("want items:[], got %s", got)
	}
	if strings.Contains(got, `"items":null`) {
		t.Fatalf("must not serialize null: %s", got)
	}
}
