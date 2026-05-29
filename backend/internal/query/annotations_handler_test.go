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
)

// fakeAnnStore is the unit-test double for annotationsStore.
type fakeAnnStore struct {
	insTenant  uuid.UUID
	insInput   AnnotationInput
	insIdemKey string
	insID      uuid.UUID
	insCreated bool
	insErr     error

	listResult []Annotation
	listErr    error
	gotType    string
	gotTarget  string
}

func (f *fakeAnnStore) Insert(_ context.Context, tenantID uuid.UUID, in AnnotationInput, idemKey string) (uuid.UUID, bool, error) {
	f.insTenant, f.insInput, f.insIdemKey = tenantID, in, idemKey
	return f.insID, f.insCreated, f.insErr
}

func (f *fakeAnnStore) List(_ context.Context, _ uuid.UUID, targetType, targetID string, _ int) ([]Annotation, error) {
	f.gotType, f.gotTarget = targetType, targetID
	return f.listResult, f.listErr
}

func postReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/annotations", strings.NewReader(body))
	return withTenantCtx(r) // tenant 1111... "acme"
}

func TestAnnotationsHandler_Create_201(t *testing.T) {
	fake := &fakeAnnStore{insID: uuid.New(), insCreated: true}
	h := NewAnnotationsHandler(fake)
	body := `{"target_type":"service","target_id":"checkout","kind":"ai_rca","payload":{"x":1},"ts":"2026-05-29T12:00:00Z"}`
	w := httptest.NewRecorder()
	h.Create(w, postReq(body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["annotation_id"] != fake.insID.String() {
		t.Fatalf("annotation_id=%q", resp["annotation_id"])
	}
	if fake.insTenant.String() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("insert used wrong tenant: %s", fake.insTenant)
	}
}

func TestAnnotationsHandler_Create_IdempotentHit_200(t *testing.T) {
	fake := &fakeAnnStore{insID: uuid.New(), insCreated: false}
	h := NewAnnotationsHandler(fake)
	body := `{"target_type":"trace","target_id":"abc","kind":"ai_rca","payload":{},"ts":"2026-05-29T12:00:00Z"}`
	r := postReq(body)
	r.Header.Set("Idempotency-Key", "k1")
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("idempotent hit must be 200, got %d", w.Code)
	}
	if fake.insIdemKey != "k1" {
		t.Fatalf("Idempotency-Key header not forwarded: %q", fake.insIdemKey)
	}
}

func TestAnnotationsHandler_Create_CrossTenant_403(t *testing.T) {
	fake := &fakeAnnStore{insID: uuid.New(), insCreated: true}
	h := NewAnnotationsHandler(fake)
	body := `{"tenant_id":"22222222-2222-2222-2222-222222222222","target_type":"service","target_id":"x","kind":"ai_rca","payload":{},"ts":"2026-05-29T12:00:00Z"}`
	w := httptest.NewRecorder()
	h.Create(w, postReq(body))
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant write must be 403, got %d", w.Code)
	}
}

func TestAnnotationsHandler_Create_BadTargetType_400(t *testing.T) {
	h := NewAnnotationsHandler(&fakeAnnStore{})
	body := `{"target_type":"galaxy","target_id":"x","kind":"ai_rca","payload":{},"ts":"2026-05-29T12:00:00Z"}`
	w := httptest.NewRecorder()
	h.Create(w, postReq(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad target_type must be 400, got %d", w.Code)
	}
}

func TestAnnotationsHandler_Create_BadTimestamp_400(t *testing.T) {
	h := NewAnnotationsHandler(&fakeAnnStore{})
	body := `{"target_type":"service","target_id":"x","kind":"ai_rca","payload":{},"ts":"not-a-time"}`
	w := httptest.NewRecorder()
	h.Create(w, postReq(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad ts must be 400, got %d", w.Code)
	}
}

func TestAnnotationsHandler_Create_NullPayload_400(t *testing.T) {
	h := NewAnnotationsHandler(&fakeAnnStore{})
	body := `{"target_type":"service","target_id":"x","kind":"ai_rca","payload":null,"ts":"2026-05-29T12:00:00Z"}`
	w := httptest.NewRecorder()
	h.Create(w, postReq(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("null payload must be 400, got %d", w.Code)
	}
}

func TestAnnotationsHandler_List_RequiresTargetType(t *testing.T) {
	h := NewAnnotationsHandler(&fakeAnnStore{})
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/annotations", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing target_type must be 400, got %d", w.Code)
	}
}

func TestAnnotationsHandler_List_ByTarget(t *testing.T) {
	fake := &fakeAnnStore{listResult: []Annotation{{Kind: "ai_rca", TargetID: "checkout"}}}
	h := NewAnnotationsHandler(fake)
	r := withTenantCtx(httptest.NewRequest(http.MethodGet, "/v1/annotations?target_type=service&target_id=checkout", nil))
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if fake.gotType != "service" || fake.gotTarget != "checkout" {
		t.Fatalf("forwarded type=%q target=%q", fake.gotType, fake.gotTarget)
	}
	var out []Annotation
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out) != 1 {
		t.Fatalf("want 1 annotation, got %d", len(out))
	}
	_ = time.Now
}
