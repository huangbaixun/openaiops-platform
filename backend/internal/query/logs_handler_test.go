package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// fakeLogsRepo is a test double for logsLister.
type fakeLogsRepo struct {
	gotParams LogsListParams
	items     []LogItem
	hasMore   bool
	err       error
}

func (f *fakeLogsRepo) List(_ context.Context, p LogsListParams) ([]LogItem, bool, error) {
	f.gotParams = p
	return f.items, f.hasMore, f.err
}

// withTenantCtx attaches a test tenant to the request context.
func withTenantCtx(r *http.Request) *http.Request {
	ctx := auth.WithTenant(r.Context(), uuid.MustParse("11111111-1111-1111-1111-111111111111"), "acme")
	return r.WithContext(ctx)
}

// ---- happy path -----------------------------------------------------------

func TestLogsHandler_List_ParsesURL(t *testing.T) {
	fake := &fakeLogsRepo{items: []LogItem{{Service: "cart", SeverityText: "ERROR"}}}
	h := NewLogsHandler(fake)

	url := "/v1/logs?service=cart&service=ship&severity=ERROR&" +
		"trace_id=4bf92f3577b34da6a3ce929d0e0e4736&limit=25"
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, url, nil))
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body=%s", w.Code, w.Body.String())
	}
	if fake.gotParams.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id: %q", fake.gotParams.TraceID)
	}
	if len(fake.gotParams.Service) != 2 {
		t.Fatalf("service slice len: %d", len(fake.gotParams.Service))
	}
	if fake.gotParams.Service[0] != "cart" || fake.gotParams.Service[1] != "ship" {
		t.Fatalf("service slice: %#v", fake.gotParams.Service)
	}
	if len(fake.gotParams.Severity) != 1 || fake.gotParams.Severity[0] != "ERROR" {
		t.Fatalf("severity slice: %#v", fake.gotParams.Severity)
	}
	if fake.gotParams.Limit != 25 {
		t.Fatalf("limit: %d", fake.gotParams.Limit)
	}

	var body LogsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items count: %d", len(body.Items))
	}
}

// ---- ts_to default lookahead (now + 1s) -----------------------------------

func TestLogsHandler_List_DefaultTsToLookahead(t *testing.T) {
	fake := &fakeLogsRepo{}
	h := NewLogsHandler(fake)

	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs", nil))
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	// ts_to should be approximately now+1s — at least 500ms in the future.
	if time.Until(fake.gotParams.TsTo) < 500*time.Millisecond {
		t.Fatalf("ts_to should be future (now+1s lookahead), got %v from now", time.Until(fake.gotParams.TsTo))
	}
}

// ---- validation: bad trace_id ---------------------------------------------

func TestLogsHandler_List_BadTraceID_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})

	for _, bad := range []string{"zzz", "tooshort", "GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG"} {
		r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?trace_id="+bad, nil))
		w := httptest.NewRecorder()
		h.List(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("trace_id=%q: want 400, got %d", bad, w.Code)
		}
	}
}

// ---- validation: bad span_id ----------------------------------------------

func TestLogsHandler_List_BadSpanID_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?span_id=nothex", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ---- validation: limit out of range ---------------------------------------

func TestLogsHandler_List_LimitTooHigh_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?limit=501", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLogsHandler_List_LimitZero_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?limit=0", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLogsHandler_List_LimitNegative_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?limit=-1", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ---- validation: bad ts_from / ts_to  -------------------------------------

func TestLogsHandler_List_BadTsFrom_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?ts_from=not-a-time", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLogsHandler_List_BadTsTo_400(t *testing.T) {
	h := NewLogsHandler(&fakeLogsRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs?ts_to=not-a-time", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ---- response shape: empty items must be [] not null ----------------------

func TestLogsHandler_List_EmptyItemsNotNull(t *testing.T) {
	fake := &fakeLogsRepo{items: make([]LogItem, 0)}
	h := NewLogsHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/logs", nil))
	w := httptest.NewRecorder()
	h.List(w, r)

	body := w.Body.String()
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("want items:[], got %s", body)
	}
	if strings.Contains(body, `"items":null`) {
		t.Fatalf("must not serialize null items: %s", body)
	}
}
