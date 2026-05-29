# PLATFORM-ASK-2 Annotations Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/specs/2026-05-29-platform-ask-2-annotations-design.md`
**Feature:** `features.json` → `PLATFORM-ASK-2`

**Goal:** Let openaiops-ai write RCA findings back to the platform via `POST /api/v1/annotations` and surface them as clickable badges on trace/service/topology nodes in the UI.

**Architecture:** A new PG `annotations` table (goose). Both `POST` and `GET /v1/annotations` live on `cmd/query` (:8081), which already holds a PG handle. Cross-tenant writes are blocked by reusing PLATFORM-ASK-1's effective-tenant-in-context (`auth.TenantID(ctx)`). Idempotency uses an unbounded `UNIQUE(tenant_id, idempotency_key)` partial index + return-existing-on-conflict. Frontend adds a shared `<AnnotationBadge>` + `useAnnotations` composable wired into three views.

**Tech Stack:** Go 1.25, chi v5, pgx v5 (`database/sql`), goose v3, dockertest; Vue 3 + TypeScript, NaiveUI, axios, Pinia, vitest, Playwright.

---

## Acceptance criteria coverage (from features.json PLATFORM-ASK-2)

| AC | Task(s) |
|----|---------|
| #1 `POST … returns 201 + annotation_id` | T1, T2, T3, T4 |
| #2 `Annotation visible as badge on trace/service/topology node` | T7, T8 |
| #3 `Cross-tenant write blocked (tenant_id must match bearer scope or X-Tenant-Id)` | T3 (403 path), T4 (integration tenant isolation) |
| #4 `Idempotency-Key header dedupes within 24h` | T2 (repo conflict path), T3 (200-on-hit) |

## File structure

**Backend (all under `backend/`):**
- Create `migrations/20260529120000_create_annotations.sql` — PG table + indexes.
- Create `internal/query/annotations_repo.go` — PG-backed repo (`*sql.DB`): `Insert`, `List`.
- Create `internal/query/annotations_repo_test.go` — dockertest integration (`//go:build integration`).
- Create `internal/query/annotations_handler.go` — `AnnotationsHandler` (`Create`, `List`).
- Create `internal/query/annotations_handler_test.go` — unit tests (fake store).
- Modify `internal/query/router.go` — add `db *sql.DB` param, register routes.
- Modify `cmd/query/main.go` — pass `db` to `query.NewRouter`.
- Modify `deploy/Caddyfile` — add `/api/v1/annotations*` handle.

**Frontend (all under `frontend/src/`):**
- Create `api/annotations.ts` — typed client (shared axios).
- Create `api/__tests__/annotations.spec.ts` — asserts shared client used.
- Create `composables/useAnnotations.ts` — fetch + state.
- Create `composables/__tests__/useAnnotations.spec.ts`.
- Create `components/AnnotationBadge.vue` — badge + detail modal.
- Create `components/__tests__/AnnotationBadge.spec.ts`.
- Modify `views/Traces/TraceDetail.vue`, `views/Services/ServiceDetail.vue`, `components/ServiceGraph/*` — wire the badge.
- Create `frontend/tests/e2e/annotations.spec.ts` — Playwright.

---

## Task 1: PG migration for `annotations` table

**Files:**
- Create: `backend/migrations/20260529120000_create_annotations.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE annotations (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target_type     TEXT         NOT NULL CHECK (target_type IN ('trace','service')),
    target_id       TEXT         NOT NULL,
    kind            TEXT         NOT NULL,
    payload         JSONB        NOT NULL,
    ts              TIMESTAMPTZ  NOT NULL,
    idempotency_key TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_annotations_lookup ON annotations(tenant_id, target_type, target_id);
CREATE UNIQUE INDEX uq_annotations_idem ON annotations(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS annotations;
```

- [ ] **Step 2: Verify it parses against the goose dialect**

The migration is applied automatically by the integration `TestMain` (goose `Up`) in Task 4. No standalone run here. Confirm the file is well-formed:

Run: `grep -c '+goose' backend/migrations/20260529120000_create_annotations.sql`
Expected: `2`

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/20260529120000_create_annotations.sql
git commit -m "feat(ask-2): annotations PG table migration"
```

---

## Task 2: Annotations PG repo (Insert with idempotency, List)

**Files:**
- Create: `backend/internal/query/annotations_repo.go`
- Test: `backend/internal/query/annotations_repo_test.go` (integration)

- [ ] **Step 1: Write the failing integration test**

`backend/internal/query/annotations_repo_test.go`:

```go
//go:build integration

package query_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

// pgForAnnotations spins PG, applies migrations, truncates, and seeds two tenants.
// It reuses the integration dockertest pattern from internal/auth/resolver_pg_test.go.
func pgForAnnotations(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	db, err := sql.Open("pgx", annotationsPGDSN)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_ = goose.SetDialect("postgres")
	require.NoError(t, goose.Up(db, "../../migrations"))
	_, err = db.ExecContext(context.Background(),
		"TRUNCATE annotations, api_keys, tenants RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	var t1, t2 string
	require.NoError(t, db.QueryRow("INSERT INTO tenants(name) VALUES('acme') RETURNING id").Scan(&t1))
	require.NoError(t, db.QueryRow("INSERT INTO tenants(name) VALUES('beta') RETURNING id").Scan(&t2))
	return db, t1, t2
}

func TestAnnotationsRepo_InsertAndList(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := query.MustUUID(t1)

	in := query.AnnotationInput{
		TargetType: "service", TargetID: "checkout", Kind: "ai_rca",
		Payload: json.RawMessage(`{"summary":"db slow"}`), TS: time.Now().UTC(),
	}
	id, created, err := repo.Insert(ctx, tid, in, "")
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, "00000000-0000-0000-0000-000000000000", id.String())

	got, err := repo.List(ctx, tid, "service", "checkout", 100)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ai_rca", got[0].Kind)
}

func TestAnnotationsRepo_IdempotencyReturnsExisting(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := query.MustUUID(t1)
	in := query.AnnotationInput{
		TargetType: "trace", TargetID: "abc123", Kind: "ai_rca",
		Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
	}
	id1, created1, err := repo.Insert(ctx, tid, in, "key-1")
	require.NoError(t, err)
	assert.True(t, created1)
	id2, created2, err := repo.Insert(ctx, tid, in, "key-1")
	require.NoError(t, err)
	assert.False(t, created2, "second insert with same key must be a dedupe hit")
	assert.Equal(t, id1, id2, "dedupe must return the same annotation id")

	got, err := repo.List(ctx, tid, "trace", "abc123", 100)
	require.NoError(t, err)
	assert.Len(t, got, 1, "only one row despite two inserts")
}

func TestAnnotationsRepo_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	db, t1, t2 := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	_, _, err := repo.Insert(ctx, query.MustUUID(t1), query.AnnotationInput{
		TargetType: "service", TargetID: "checkout", Kind: "ai_rca",
		Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
	}, "")
	require.NoError(t, err)

	// tenant 2 must see nothing
	got, err := repo.List(ctx, query.MustUUID(t2), "service", "checkout", 100)
	require.NoError(t, err)
	assert.Empty(t, got, "tenant B must not see tenant A's annotation")
}
```

This test needs an `annotationsPGDSN` var + a `TestMain` for PG. The query package's existing integration `TestMain` (in `test_helpers_test.go`) starts **ClickHouse**, not PG. To avoid two `TestMain`s in the same `query_test` package, add a PG fixture into the existing `test_helpers_test.go` `TestMain` (Step 1b).

- [ ] **Step 1b: Extend the integration `TestMain` to also start PG**

Modify `backend/internal/query/test_helpers_test.go` — add a PG container alongside the CH fixture:

```go
// added imports:
//   "database/sql"
//   "fmt"
//   _ "github.com/jackc/pgx/v5/stdlib"
//   "github.com/ory/dockertest/v3"
//   "github.com/ory/dockertest/v3/docker"

var annotationsPGDSN string

// startPG is called from TestMain to provide a PG instance for the
// annotations repo integration tests (the rest of the package uses CH).
func startPG() func() {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("dockertest pool: %v", err)
	}
	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres", Tag: "16-alpine",
		Env: []string{"POSTGRES_PASSWORD=test", "POSTGRES_DB=test"},
	}, func(c *docker.HostConfig) { c.AutoRemove = true })
	if err != nil {
		log.Fatalf("dockertest pg run: %v", err)
	}
	annotationsPGDSN = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable",
		res.GetPort("5432/tcp"))
	if err := pool.Retry(func() error {
		db, _ := sql.Open("pgx", annotationsPGDSN)
		defer db.Close()
		return db.Ping()
	}); err != nil {
		log.Fatalf("pg not ready: %v", err)
	}
	return func() { _ = pool.Purge(res) }
}
```

Then in the existing `TestMain`, wrap the run:

```go
func TestMain(m *testing.M) {
	fixture := chtest.StartCH(fatalReporter{},
		"20260525120000_create_traces_v1.sql",
		"20260527120000_create_logs_v1.sql",
		"20260527120000_create_topology_edges_v1.sql",
		"20260527120100_create_service_stats_v1.sql",
	)
	dsn = fixture.DSN
	stopPG := startPG()

	code := m.Run()
	stopPG()
	_ = fixture.Close()
	os.Exit(code)
}
```

- [ ] **Step 2: Run the test to verify it fails (compile error: undefined symbols)**

Run: `cd backend && go test -tags=integration -run TestAnnotationsRepo ./internal/query/ 2>&1 | head -20`
Expected: build failure — `undefined: query.NewAnnotationsRepo`, `query.AnnotationInput`, `query.MustUUID`.

> NOTE: if the docker daemon is down (as on 2026-05-29), this command cannot run. In that case verify compilation only with `go test -tags=integration -run xxx_none -c -o /dev/null ./internal/query/` after Step 3, and land the feature `done_with_concerns` with a drift note (mirror ASK-1's D7).

- [ ] **Step 3: Write the repo implementation**

`backend/internal/query/annotations_repo.go`:

```go
package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Annotation is one stored annotation row.
type Annotation struct {
	ID         uuid.UUID       `json:"id"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	TS         time.Time       `json:"ts"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AnnotationInput is the writable subset (tenant comes from ctx, never the body).
type AnnotationInput struct {
	TargetType string
	TargetID   string
	Kind       string
	Payload    json.RawMessage
	TS         time.Time
}

// AnnotationsRepo is a PG-backed store. Unlike the CH repos in this package it
// takes *sql.DB; annotations are low-volume relational metadata (see spec §1).
type AnnotationsRepo struct{ db *sql.DB }

func NewAnnotationsRepo(db *sql.DB) *AnnotationsRepo { return &AnnotationsRepo{db: db} }

// MustUUID is a tiny helper used by tests and callers that already validated input.
func MustUUID(s string) uuid.UUID { return uuid.MustParse(s) }

// Insert writes an annotation scoped to tenantID. When idemKey != "" it dedupes
// on (tenant_id, idempotency_key): a repeated key returns the existing id with
// created=false. tenantID always wins over any body-supplied tenant.
func (r *AnnotationsRepo) Insert(ctx context.Context, tenantID uuid.UUID, in AnnotationInput, idemKey string) (uuid.UUID, bool, error) {
	const ins = `
		INSERT INTO annotations(tenant_id, target_type, target_id, kind, payload, ts, idempotency_key)
		VALUES($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		ON CONFLICT (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING id`
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, ins,
		tenantID, in.TargetType, in.TargetID, in.Kind, []byte(in.Payload), in.TS, idemKey,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, err
	}
	// Conflict on a non-empty idempotency key: return the existing row.
	const sel = `SELECT id FROM annotations WHERE tenant_id = $1 AND idempotency_key = $2`
	if err := r.db.QueryRowContext(ctx, sel, tenantID, idemKey).Scan(&id); err != nil {
		return uuid.Nil, false, err
	}
	return id, false, nil
}

// List returns annotations for tenantID + targetType, optionally narrowed to a
// single targetID, newest first.
func (r *AnnotationsRepo) List(ctx context.Context, tenantID uuid.UUID, targetType, targetID string, limit int) ([]Annotation, error) {
	q := `
		SELECT id, target_type, target_id, kind, payload, ts, created_at
		FROM annotations
		WHERE tenant_id = $1 AND target_type = $2`
	args := []any{tenantID, targetType}
	if targetID != "" {
		q += ` AND target_id = $3`
		args = append(args, targetID)
	}
	q += ` ORDER BY ts DESC LIMIT ` + itoa(limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Annotation{}
	for rows.Next() {
		var a Annotation
		var payload []byte
		if err := rows.Scan(&a.ID, &a.TargetType, &a.TargetID, &a.Kind, &payload, &a.TS, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Payload = json.RawMessage(payload)
		out = append(out, a)
	}
	return out, rows.Err()
}
```

Add the `itoa` helper at the bottom of the file (limit is server-validated to [1,500] in the handler, so string concatenation is injection-safe — but keep it a dedicated helper so the intent is explicit):

```go
func itoa(n int) string { return strconv.Itoa(n) }
```

…and add `"strconv"` to the import block.

- [ ] **Step 4: Run the integration test to verify it passes**

Run: `cd backend && go test -tags=integration -timeout 240s -run TestAnnotationsRepo ./internal/query/ -v`
Expected: `PASS` for `InsertAndList`, `IdempotencyReturnsExisting`, `TenantIsolation`.
(If docker is down: `go test -tags=integration -run xxx_none -c -o /dev/null ./internal/query/` must compile cleanly.)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/query/annotations_repo.go backend/internal/query/annotations_repo_test.go backend/internal/query/test_helpers_test.go
git commit -m "feat(ask-2): annotations PG repo with idempotency + tenant isolation"
```

---

## Task 3: Annotations HTTP handler (POST create, GET list)

**Files:**
- Create: `backend/internal/query/annotations_handler.go`
- Test: `backend/internal/query/annotations_handler_test.go`

- [ ] **Step 1: Write the failing unit test**

`backend/internal/query/annotations_handler_test.go`:

```go
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
	insTenant   uuid.UUID
	insInput    AnnotationInput
	insIdemKey  string
	insID       uuid.UUID
	insCreated  bool
	insErr      error

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
	// body tenant_id != ctx tenant (1111...) -> 403, repo never called
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd backend && go test -run TestAnnotationsHandler ./internal/query/ 2>&1 | head -20`
Expected: build failure — `undefined: NewAnnotationsHandler`, `annotationsStore`.

- [ ] **Step 3: Write the handler**

`backend/internal/query/annotations_handler.go`:

```go
package query

import (
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
	if req.TargetID == "" || req.Kind == "" || len(req.Payload) == 0 {
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd backend && go test -run TestAnnotationsHandler ./internal/query/ -v`
Expected: all 7 subtests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/query/annotations_handler.go backend/internal/query/annotations_handler_test.go
git commit -m "feat(ask-2): annotations HTTP handler (201/200/403/400 + list)"
```

---

## Task 4: Wire routes + main + Caddy

**Files:**
- Modify: `backend/internal/query/router.go`
- Modify: `backend/cmd/query/main.go`
- Modify: `deploy/Caddyfile`

- [ ] **Step 1: Add `db` param + routes to the query router**

In `backend/internal/query/router.go`, change the signature and register the routes inside the authed group:

```go
// import "database/sql" at top of file

func NewRouter(resolver auth.Resolver, ch *chquery.Conn, db *sql.DB) *chi.Mux {
	// ... unchanged setup ...
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(resolver))

		// ... existing traces/logs/services/topology registrations ...

		// PLATFORM-ASK-2: annotations write-back (PG-backed; see spec ADR-0003 deviation).
		ah := NewAnnotationsHandler(NewAnnotationsRepo(db))
		r.Post("/v1/annotations", ah.Create)
		r.Get("/v1/annotations", ah.List)
	})
	return r
}
```

- [ ] **Step 2: Pass `db` from `cmd/query/main.go`**

In `backend/cmd/query/main.go`, the `db` handle already exists (used for the resolver). Update the call:

```go
	router := query.NewRouter(resolver, ch, db)
```

- [ ] **Step 3: Build to verify wiring compiles**

Run: `cd backend && go build ./... && go test ./internal/query/ 2>&1 | tail -5`
Expected: build OK; unit tests PASS.

- [ ] **Step 4: Add the Caddy handle**

In `deploy/Caddyfile`, add **before** the `handle /api/* { … gateway }` catch-all (mirror the SLICE-3 one-liners):

```
	handle /api/v1/annotations* {
		uri strip_prefix /api
		reverse_proxy query:8081
	}
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/query/router.go backend/cmd/query/main.go deploy/Caddyfile
git commit -m "feat(ask-2): wire annotations routes on query + Caddy handle"
```

---

## Task 5: Frontend API module `annotations.ts`

**Files:**
- Create: `frontend/src/api/annotations.ts`
- Test: `frontend/src/api/__tests__/annotations.spec.ts`

- [ ] **Step 1: Write the failing spec (asserts shared client — SLICE-3 T15 regression guard)**

`frontend/src/api/__tests__/annotations.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../client', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

import client from '../client'
import { fetchAnnotations, createAnnotation } from '../annotations'

const mockGet = client.get as ReturnType<typeof vi.fn>
const mockPost = client.post as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('fetchAnnotations', () => {
  it('routes through shared axios client at /v1/annotations (NOT raw fetch)', async () => {
    mockGet.mockResolvedValue({ data: [] })
    await fetchAnnotations('service', { targetId: 'checkout' })
    expect(mockGet).toHaveBeenCalledWith('/v1/annotations', {
      params: { target_type: 'service', target_id: 'checkout' },
    })
  })

  it('omits target_id when not given', async () => {
    mockGet.mockResolvedValue({ data: [] })
    await fetchAnnotations('service')
    expect(mockGet).toHaveBeenCalledWith('/v1/annotations', {
      params: { target_type: 'service' },
    })
  })
})

describe('createAnnotation', () => {
  it('POSTs through the shared client and returns annotation_id', async () => {
    mockPost.mockResolvedValue({ data: { annotation_id: 'abc' } })
    const id = await createAnnotation({
      target_type: 'trace', target_id: 't1', kind: 'ai_rca',
      payload: { x: 1 }, ts: '2026-05-29T12:00:00Z',
    })
    expect(mockPost).toHaveBeenCalledWith('/v1/annotations', expect.objectContaining({
      target_type: 'trace', target_id: 't1',
    }))
    expect(id).toBe('abc')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend && npx vitest run src/api/__tests__/annotations.spec.ts`
Expected: FAIL — cannot resolve `../annotations`.

- [ ] **Step 3: Write the module**

`frontend/src/api/annotations.ts`:

```ts
import client from './client'

export type AnnotationTargetType = 'trace' | 'service'

export interface Annotation {
  id: string
  target_type: AnnotationTargetType
  target_id: string
  kind: string
  payload: Record<string, unknown>
  ts: string
  created_at: string
}

export interface CreateAnnotationInput {
  target_type: AnnotationTargetType
  target_id: string
  kind: string
  payload: Record<string, unknown>
  ts: string
}

// Uses the shared axios client (api/client.ts) so the Bearer interceptor runs.
// Raw fetch() here would skip auth — the exact SLICE-3 T15 regression.
export async function fetchAnnotations(
  targetType: AnnotationTargetType,
  opts: { targetId?: string; limit?: number } = {},
): Promise<Annotation[]> {
  const params: Record<string, string | number> = { target_type: targetType }
  if (opts.targetId) params.target_id = opts.targetId
  if (opts.limit) params.limit = opts.limit
  const { data } = await client.get<Annotation[]>('/v1/annotations', { params })
  return data
}

export async function createAnnotation(input: CreateAnnotationInput): Promise<string> {
  const { data } = await client.post<{ annotation_id: string }>('/v1/annotations', input)
  return data.annotation_id
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend && npx vitest run src/api/__tests__/annotations.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/annotations.ts frontend/src/api/__tests__/annotations.spec.ts
git commit -m "feat(ask-2): frontend annotations api module (shared client)"
```

---

## Task 6: `useAnnotations` composable

**Files:**
- Create: `frontend/src/composables/useAnnotations.ts`
- Test: `frontend/src/composables/__tests__/useAnnotations.spec.ts`

- [ ] **Step 1: Write the failing spec**

`frontend/src/composables/__tests__/useAnnotations.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'

vi.mock('../../api/annotations', () => ({
  fetchAnnotations: vi.fn(),
}))

import { fetchAnnotations } from '../../api/annotations'
import { useAnnotations } from '../useAnnotations'

const mockFetch = fetchAnnotations as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('useAnnotations', () => {
  it('loads annotations for a target on creation', async () => {
    mockFetch.mockResolvedValue([{ id: '1', kind: 'ai_rca', target_id: 'checkout' }])
    const { annotations, loading } = useAnnotations('service', () => 'checkout')
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith('service', { targetId: 'checkout' })
    expect(annotations.value).toHaveLength(1)
    expect(loading.value).toBe(false)
  })

  it('captures errors', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))
    const { error } = useAnnotations('trace', () => 't1')
    await flushPromises()
    expect(error.value).toBe('boom')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend && npx vitest run src/composables/__tests__/useAnnotations.spec.ts`
Expected: FAIL — cannot resolve `../useAnnotations`.

- [ ] **Step 3: Write the composable**

`frontend/src/composables/useAnnotations.ts`:

```ts
import { ref, watchEffect } from 'vue'
import {
  fetchAnnotations,
  type Annotation,
  type AnnotationTargetType,
} from '../api/annotations'

// useAnnotations loads annotations for a target. targetId is a getter so the
// caller can pass a reactive route param; when it returns undefined the call is
// skipped (used by pages that fetch all-of-type via fetchAll instead).
export function useAnnotations(
  targetType: AnnotationTargetType,
  targetId: () => string | undefined,
) {
  const annotations = ref<Annotation[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(): Promise<void> {
    const id = targetId()
    loading.value = true
    error.value = null
    try {
      annotations.value = await fetchAnnotations(targetType, id ? { targetId: id } : {})
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  watchEffect(() => {
    void targetId() // track dependency
    void load()
  })

  return { annotations, loading, error, reload: load }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend && npx vitest run src/composables/__tests__/useAnnotations.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/composables/useAnnotations.ts frontend/src/composables/__tests__/useAnnotations.spec.ts
git commit -m "feat(ask-2): useAnnotations composable"
```

---

## Task 7: `<AnnotationBadge>` component

**Files:**
- Create: `frontend/src/components/AnnotationBadge.vue`
- Test: `frontend/src/components/__tests__/AnnotationBadge.spec.ts`

- [ ] **Step 1: Write the failing spec**

`frontend/src/components/__tests__/AnnotationBadge.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AnnotationBadge from '../AnnotationBadge.vue'

const sample = [
  { id: '1', target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
    payload: { summary: 'db slow' }, ts: '2026-05-29T12:00:00Z', created_at: '2026-05-29T12:00:01Z' },
]

describe('AnnotationBadge', () => {
  it('renders nothing when there are no annotations', () => {
    const w = mount(AnnotationBadge, { props: { annotations: [] } })
    expect(w.find('[data-testid="annotation-badge"]').exists()).toBe(false)
  })

  it('shows the count when annotations exist', () => {
    const w = mount(AnnotationBadge, { props: { annotations: sample } })
    const badge = w.find('[data-testid="annotation-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('1')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend && npx vitest run src/components/__tests__/AnnotationBadge.spec.ts`
Expected: FAIL — cannot resolve `../AnnotationBadge.vue`.

- [ ] **Step 3: Write the component** (NaiveUI, follows OpenAPM hi-fi design — small RCA-tagged badge that opens a detail modal)

`frontend/src/components/AnnotationBadge.vue`:

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import { NTag, NModal, NCard, NList, NListItem, NCode } from 'naive-ui'
import type { Annotation } from '../api/annotations'

const props = defineProps<{ annotations: Annotation[] }>()
const show = ref(false)
const count = computed(() => props.annotations.length)

function pretty(payload: Record<string, unknown>): string {
  return JSON.stringify(payload, null, 2)
}
</script>

<template>
  <span v-if="count > 0">
    <NTag
      data-testid="annotation-badge"
      type="warning"
      size="small"
      round
      style="cursor: pointer"
      @click="show = true"
    >
      🤖 AI · {{ count }}
    </NTag>

    <NModal v-model:show="show">
      <NCard
        style="width: 600px; max-width: 90vw"
        title="AI annotations"
        :bordered="false"
        size="huge"
        role="dialog"
        aria-modal="true"
      >
        <NList>
          <NListItem v-for="a in props.annotations" :key="a.id">
            <div>
              <NTag size="tiny" type="info" round>{{ a.kind }}</NTag>
              <span style="margin-left: 8px; opacity: 0.7">{{ a.ts }}</span>
            </div>
            <NCode :code="pretty(a.payload)" language="json" />
          </NListItem>
        </NList>
      </NCard>
    </NModal>
  </span>
</template>
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend && npx vitest run src/components/__tests__/AnnotationBadge.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/AnnotationBadge.vue frontend/src/components/__tests__/AnnotationBadge.spec.ts
git commit -m "feat(ask-2): AnnotationBadge component with detail modal"
```

---

## Task 8: Wire badge into 3 views + Playwright e2e

**Files:**
- Modify: `frontend/src/views/Traces/TraceDetail.vue`
- Modify: `frontend/src/views/Services/ServiceDetail.vue`
- Modify: `frontend/src/components/ServiceGraph/` (node decoration — inspect the dir to find the node render site)
- Create: `frontend/tests/e2e/annotations.spec.ts`

- [ ] **Step 1: Wire into TraceDetail header**

In `frontend/src/views/Traces/TraceDetail.vue` `<script setup>`, after the existing trace-id resolution (the route param used to fetch the trace):

```ts
import AnnotationBadge from '../../components/AnnotationBadge.vue'
import { useAnnotations } from '../../composables/useAnnotations'

// traceId is the existing ref/computed for the current trace id in this view.
const { annotations: traceAnnotations } = useAnnotations('trace', () => traceId.value)
```

In the template, next to the trace title/header element:

```vue
<AnnotationBadge :annotations="traceAnnotations" />
```

- [ ] **Step 2: Wire into ServiceDetail**

In `frontend/src/views/Services/ServiceDetail.vue` `<script setup>` (the view already resolves a `name` route param for the service):

```ts
import AnnotationBadge from '../../components/AnnotationBadge.vue'
import { useAnnotations } from '../../composables/useAnnotations'

const { annotations: svcAnnotations } = useAnnotations('service', () => name.value)
```

Template, next to the service-name heading:

```vue
<AnnotationBadge :annotations="svcAnnotations" />
```

- [ ] **Step 3: Wire into the topology graph node**

Inspect the graph node render site:

Run: `ls frontend/src/components/ServiceGraph/ && grep -rln "service" frontend/src/components/ServiceGraph/`

In the parent page that renders `<ServiceGraph>` (e.g. `views/Topology/TopologyPage.vue` and `views/Overview/`), fetch all service annotations once and pass a per-service count map down:

```ts
import { ref, onMounted } from 'vue'
import { fetchAnnotations, type Annotation } from '../../api/annotations'

const annByService = ref<Record<string, Annotation[]>>({})
onMounted(async () => {
  const all = await fetchAnnotations('service')
  annByService.value = all.reduce<Record<string, Annotation[]>>((acc, a) => {
    ;(acc[a.target_id] ??= []).push(a)
    return acc
  }, {})
})
```

Pass `:ann-by-service="annByService"` into `<ServiceGraph>` and, in the node template inside ServiceGraph, render a small dot when `annByService[node.service]?.length`. Keep this minimal — a `<title>`/tooltip with the count is sufficient for the node decoration; the full modal lives on the detail pages.

- [ ] **Step 4: Run the frontend unit + build to verify no regressions**

Run: `cd frontend && npm run build && npx vitest run`
Expected: build succeeds; all vitest suites pass (existing + the 8 new tests from T5–T7).

- [ ] **Step 5: Write the Playwright e2e**

`frontend/tests/e2e/annotations.spec.ts` (relative paths only + baseURL from `playwright.config.ts` — SLICE-2 T14 lesson; never hardcode a port):

```ts
import { test, expect } from '@playwright/test'

// Seeds an annotation via the API using the dev acme key, then asserts the
// badge renders on the service detail page. Assumes `make up && make seed`
// has run and the acme key is present (deploy/seed.sql).
test('AI annotation badge appears on service detail', async ({ page, request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: {
      target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
      payload: { summary: 'e2e seeded' }, ts: new Date().toISOString(),
    },
  })
  expect([200, 201]).toContain(res.status())

  await page.addInitScript(() => localStorage.setItem('apiKey', 'test-key-acme'))
  await page.goto('/services/checkout')
  await expect(page.getByTestId('annotation-badge')).toBeVisible()
})

test('cross-tenant write is blocked (403)', async ({ request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: {
      tenant_id: '22222222-2222-2222-2222-222222222222', // beta, not acme
      target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
      payload: {}, ts: new Date().toISOString(),
    },
  })
  expect(res.status()).toBe(403)
})
```

- [ ] **Step 6: Run the e2e (requires stack up + seed)**

Run: `cd deploy && make up && make seed` then `cd frontend && npx playwright test annotations.spec.ts`
Expected: both tests PASS. (If docker is down, this is deferred to the verification gate; note it.)

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/Traces/TraceDetail.vue frontend/src/views/Services/ServiceDetail.vue frontend/src/components/ServiceGraph frontend/src/views/Topology frontend/src/views/Overview frontend/tests/e2e/annotations.spec.ts
git commit -m "feat(ask-2): wire AnnotationBadge into trace/service/topology + e2e"
```

---

## Final verification (verification-before-completion)

- [ ] `cd backend && go vet ./... && go test -count=1 ./...` — all PASS.
- [ ] `cd backend && go test -tags=integration -timeout 240s ./internal/query/` — PASS (or compile-only + drift note if docker down).
- [ ] `cd backend && make lint-ch` — annotations repo uses PG (`*sql.DB`), not CH, so it must not trip the bare-CH lint. Confirm clean.
- [ ] `cd frontend && npm run build && npx vitest run` — PASS.
- [ ] `cd frontend && npx playwright test` — PASS (stack up + seed) or deferred with drift note.
- [ ] Update `features.json` PLATFORM-ASK-2 → `done` (or `done_with_concerns` if integration/e2e deferred) with an `evidence` field; add `recently_completed` + any `known_drift` to `docs/claude-progress.json`.
- [ ] Append a one-line amendment pointer to `docs/decisions/0003-query-api-deployment-shape.md` noting query's first write endpoint (annotations) per the spec deviation.

## Notes / known traps applied from lessons-learned

- **SLICE-3 T15:** every new `api/` module routes through the shared `client.ts`; specs assert `client.get/post`, not raw URLs. (T5)
- **SLICE-2 T14:** Playwright specs use relative paths + `baseURL`; never hardcode a port. (T8)
- **CLAUDE.md:** PG schema change ⇒ goose migration with `+goose Up/Down` (T1); bcrypt-touching tests need `-timeout` ≥ 60s — N/A here (no new bcrypt paths).
- **Multi-container staleness (SLICE-3 T15):** before e2e, `docker compose build query frontend && up -d` so the running stack has the new code.
```
