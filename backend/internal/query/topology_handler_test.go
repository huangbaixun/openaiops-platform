package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeTopologyRepo is the unit-test double for topologyRepoIface.
type fakeTopologyRepo struct {
	gotWindow string
	gotLimit  int
	resp      *TopologyResponse
	err       error
}

func (f *fakeTopologyRepo) Topology(_ context.Context, window string, nodeLimit int) (*TopologyResponse, error) {
	f.gotWindow = window
	f.gotLimit = nodeLimit
	return f.resp, f.err
}

// ---- Defaults --------------------------------------------------------------

func TestTopologyHandler_Get_Defaults(t *testing.T) {
	fake := &fakeTopologyRepo{resp: &TopologyResponse{Window: "1h"}}
	h := NewTopologyHandler(fake)

	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if fake.gotWindow != "1h" || fake.gotLimit != 100 {
		t.Fatalf("defaults: window=%q limit=%d", fake.gotWindow, fake.gotLimit)
	}
}

func TestTopologyHandler_Get_ParsesQuery(t *testing.T) {
	fake := &fakeTopologyRepo{resp: &TopologyResponse{}}
	h := NewTopologyHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology?window=24h&node_limit=42", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	if fake.gotWindow != "24h" || fake.gotLimit != 42 {
		t.Fatalf("parsed: window=%q limit=%d", fake.gotWindow, fake.gotLimit)
	}
}

// ---- Validation 400 -------------------------------------------------------

func TestTopologyHandler_Get_BadWindow_400(t *testing.T) {
	h := NewTopologyHandler(&fakeTopologyRepo{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology?window=lifetime", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestTopologyHandler_Get_BadNodeLimit_400(t *testing.T) {
	h := NewTopologyHandler(&fakeTopologyRepo{})
	for _, bad := range []string{"0", "-3", "301", "abc"} {
		r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology?node_limit="+bad, nil))
		w := httptest.NewRecorder()
		h.Get(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("node_limit=%q: want 400 got %d", bad, w.Code)
		}
	}
}

// ---- Empty nodes/edges serialize as [] ------------------------------------

func TestTopologyHandler_Get_EmptyNodesEdgesNotNull(t *testing.T) {
	fake := &fakeTopologyRepo{resp: &TopologyResponse{Window: "1h"}}
	h := NewTopologyHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)
	body := w.Body.String()
	if !strings.Contains(body, `"nodes":[]`) || !strings.Contains(body, `"edges":[]`) {
		t.Fatalf("want nodes:[] and edges:[], got %s", body)
	}
}

// ---- Repo error -> 500 ----------------------------------------------------

func TestTopologyHandler_Get_RepoErr_500(t *testing.T) {
	fake := &fakeTopologyRepo{err: errors.New("ch down")}
	h := NewTopologyHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", w.Code)
	}
}

// ---- Response shape -------------------------------------------------------

func TestTopologyHandler_Get_ResponseShape(t *testing.T) {
	fake := &fakeTopologyRepo{
		resp: &TopologyResponse{
			Window: "1h",
			Nodes:  []TopologyNode{{Service: "checkout", Kind: "service", Calls: 5}},
			Edges:  []TopologyEdge{{Caller: "checkout", Callee: "redis", CalleeKind: "external", Calls: 2}},
		},
	}
	h := NewTopologyHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/topology", nil))
	w := httptest.NewRecorder()
	h.Get(w, r)
	var body TopologyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Nodes) != 1 || len(body.Edges) != 1 {
		t.Fatalf("shape: %#v", body)
	}
	if body.Nodes[0].Service != "checkout" || body.Edges[0].CalleeKind != "external" {
		t.Fatalf("content: %#v", body)
	}
}
