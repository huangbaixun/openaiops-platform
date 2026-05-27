package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// fakeServicesRepo is the unit-test double for servicesRepoIface.
type fakeServicesRepo struct {
	gotListWindow string
	gotListLimit  int
	gotListSort   string
	listItems     []ServicesListItem
	listErr       error

	gotDetailName   string
	gotDetailWindow string
	detailResp      *ServiceDetailResponse
	detailErr       error
}

func (f *fakeServicesRepo) List(_ context.Context, window string, limit int, sort string) ([]ServicesListItem, error) {
	f.gotListWindow = window
	f.gotListLimit = limit
	f.gotListSort = sort
	return f.listItems, f.listErr
}

func (f *fakeServicesRepo) Detail(_ context.Context, name, window string) (*ServiceDetailResponse, error) {
	f.gotDetailName = name
	f.gotDetailWindow = window
	return f.detailResp, f.detailErr
}

// ---- List: happy path ------------------------------------------------------

func TestServicesHandler_List_Defaults(t *testing.T) {
	fake := &fakeServicesRepo{listItems: []ServicesListItem{{Service: "checkout", InboundCalls: 5}}}
	h := NewServicesHandler(fake)

	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services", nil))
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body=%s", w.Code, w.Body.String())
	}
	if fake.gotListWindow != "1h" || fake.gotListLimit != 100 || fake.gotListSort != "calls" {
		t.Fatalf("defaults: window=%q limit=%d sort=%q", fake.gotListWindow, fake.gotListLimit, fake.gotListSort)
	}
	var body ServicesListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Window != "1h" || len(body.Items) != 1 || body.Items[0].Service != "checkout" {
		t.Fatalf("body: %#v", body)
	}
}

func TestServicesHandler_List_ParsesQuery(t *testing.T) {
	fake := &fakeServicesRepo{}
	h := NewServicesHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services?window=6h&limit=42&sort=errors", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if fake.gotListWindow != "6h" || fake.gotListLimit != 42 || fake.gotListSort != "errors" {
		t.Fatalf("parsed: window=%q limit=%d sort=%q", fake.gotListWindow, fake.gotListLimit, fake.gotListSort)
	}
}

// ---- List: validation 400s -------------------------------------------------

func TestServicesHandler_List_BadWindow_400(t *testing.T) {
	h := NewServicesHandler(&fakeServicesRepo{})
	for _, bad := range []string{"5m", "2h", "garbage", "1H"} {
		r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services?window="+bad, nil))
		w := httptest.NewRecorder()
		h.List(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("window=%q: want 400 got %d", bad, w.Code)
		}
	}
}

func TestServicesHandler_List_BadSort_400(t *testing.T) {
	h := NewServicesHandler(&fakeServicesRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services?sort=latency", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestServicesHandler_List_BadLimit_400(t *testing.T) {
	h := NewServicesHandler(&fakeServicesRepo{})
	for _, bad := range []string{"0", "-1", "501", "notanint"} {
		r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services?limit="+bad, nil))
		w := httptest.NewRecorder()
		h.List(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("limit=%q: want 400 got %d", bad, w.Code)
		}
	}
}

// ---- List: empty items must serialize as [] not null ----------------------

func TestServicesHandler_List_EmptyItemsNotNull(t *testing.T) {
	fake := &fakeServicesRepo{listItems: nil}
	h := NewServicesHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("want items:[], got %s", body)
	}
}

// ---- List: repo error -> 500 ----------------------------------------------

func TestServicesHandler_List_RepoErr_500(t *testing.T) {
	fake := &fakeServicesRepo{listErr: errors.New("ch down")}
	h := NewServicesHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/services", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", w.Code)
	}
}

// ---- Detail ----------------------------------------------------------------

// detailReq builds an httptest request with chi URLParam "name" pre-populated.
func detailReq(name, rawQuery string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/services/"+name+"?"+rawQuery, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return withTenantCtx(r)
}

func TestServicesHandler_Detail_404_NilResp(t *testing.T) {
	fake := &fakeServicesRepo{detailResp: nil}
	h := NewServicesHandler(fake)
	w := httptest.NewRecorder()
	h.Detail(w, detailReq("ghost", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

func TestServicesHandler_Detail_Happy_200(t *testing.T) {
	resp := &ServiceDetailResponse{Service: "checkout", Window: "1h"}
	resp.Stats.Inbound = ServiceDirectionStats{Calls: 5}
	fake := &fakeServicesRepo{detailResp: resp}
	h := NewServicesHandler(fake)
	w := httptest.NewRecorder()
	h.Detail(w, detailReq("checkout", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if fake.gotDetailName != "checkout" || fake.gotDetailWindow != "1h" {
		t.Fatalf("repo called with name=%q window=%q", fake.gotDetailName, fake.gotDetailWindow)
	}
}

func TestServicesHandler_Detail_BadWindow_400(t *testing.T) {
	h := NewServicesHandler(&fakeServicesRepo{})
	w := httptest.NewRecorder()
	h.Detail(w, detailReq("checkout", "window=2h"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestServicesHandler_Detail_RepoErr_500(t *testing.T) {
	fake := &fakeServicesRepo{detailErr: errors.New("ch down")}
	h := NewServicesHandler(fake)
	w := httptest.NewRecorder()
	h.Detail(w, detailReq("checkout", ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", w.Code)
	}
}
