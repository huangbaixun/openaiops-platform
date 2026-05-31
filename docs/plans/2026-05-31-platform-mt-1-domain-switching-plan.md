# PLATFORM-MT-1 — Domain-scoped Multi-tenant Switching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use harness:subagent-driven-development (recommended) or harness:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/specs/2026-05-31-platform-mt-1-domain-switching-design.md` (feature `PLATFORM-MT-1`). **ADR:** `docs/decisions/0004-domain-scoped-tenant-switching.md`.

**Goal:** Let a `domain`-scoped API key view and switch among the tenants of its own domain (via the generalized `X-Tenant-Id` header), wiring the PLATFORM-UI-1 ScopePill to real switching — without weakening the one-active-tenant-per-request isolation invariant.

**Architecture:** Add PG `domains` + `tenants.domain_id/environment` + `audit_log`. Generalize the ASK-1 `service:ai` X-Tenant-Id branch with a new `domain` scope validated by shared non-NULL `domain_id` (fail-closed). New gateway endpoints `GET /api/v1/tenants` (enumerate domain peers) + `POST /api/v1/tenants/switch` (validate + audit once). Frontend store holds `activeTenantId`; axios attaches `X-Tenant-Id`; ScopePill becomes a live switcher. The data layer (`MustTenantScope` + Row Policy) is untouched — switching happens only at auth time.

**Tech Stack:** Go 1.25 (chi v5, database/sql + pgx stdlib, goose v3, dockertest, google/uuid), Vue 3 + TS + Pinia + NaiveUI + vitest/Playwright.

---

## Acceptance-criteria → task traceability

| AC | Criterion (abridged) | Task(s) |
|---|---|---|
| 1 | domains table + tenants.domain_id/environment migrated (up/down); existing tenants NULL domain_id | T1 |
| 2 | `domain` key switches within its domain; out-of-domain → 403; absent header → own; read-write/service:ai unchanged | T3 |
| 3 | GET /api/v1/tenants (peers/single); POST /api/v1/tenants/switch (validate + audit + 200/403) | T4 |
| 4 | ScopePill wired: Project dropdown switches active tenant (no re-login); Env shows/groups; pages re-query | T5, T6 |
| 5 | Isolation preserved: reverse E2E (in-domain sees only target; out-of-domain blocked; A-writes/B-reads→0) | T3, T7 |
| 6 | ADR-0004 written (done); each switch writes an audit_log row | T1 (table), T4 (write) |
| 7 | All existing tests green; new unit/integration/e2e pass | T7 (gate); every task |

No orphan criteria. No task touches `out_of_scope` (no human users/sessions, no env access-control, no domain CRUD UI, no cross-domain switching).

---

## File structure

**Backend — new:**
- `backend/migrations/20260531120000_create_domains_and_audit.sql` — domains, tenants columns, audit_log.
- `backend/internal/identity/tenants_repo.go` — PG reads: domain peers, single tenant, tenant-with-domain; audit insert.
- `backend/internal/identity/tenants_handler.go` — `GET /api/v1/tenants` + `POST /api/v1/tenants/switch`.
- `backend/internal/identity/*_test.go` — handler unit tests (fake store) + repo integration test.

**Backend — modified:**
- `backend/internal/tenant/tenant.go` — add `DomainID uuid.UUID` + `Environment string`.
- `backend/internal/auth/resolver_pg.go` — select domain_id/environment in ResolveBearer + TenantByID.
- `backend/internal/auth/ctx.go` — add `WithScope/Scope` + `WithKeyID/KeyID`.
- `backend/internal/auth/middleware.go` — add `ScopeDomain` + domain branch + `tenantInDomain`; stash scope + key id.
- `backend/internal/httpsrv/server.go` — `NewRouter(resolver, db)`, mount the two tenants routes.
- `backend/cmd/gateway/main.go` — pass `db` to `NewRouter`.
- `deploy/seed.sql` — a demo domain + two env-tagged tenants + a `domain`-scoped key.

**Frontend — new:**
- `frontend/src/api/tenants.ts` — `fetchTenants()`, `switchTenant(id)`.

**Frontend — modified:**
- `frontend/src/stores/auth.ts` — `activeTenantId`, `domainTenants`, fetch on login/restore, `switchActiveTenant`.
- `frontend/src/api/client.ts` — attach `X-Tenant-Id` from active selection.
- `frontend/src/components/ScopePill.vue` — live Project dropdown + Env grouping (replaces UI-1 static stub).
- `frontend/e2e/tenant-switch.spec.ts` — new e2e.

**Untouched:** all `chquery`, query handlers, ingester, Row Policy DDL — the isolation mechanism does not change.

---

## Task 1: Migration — domains, tenant columns, audit_log

AC#1, AC#6 (table).

**Files:**
- Create: `backend/migrations/20260531120000_create_domains_and_audit.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tenants ADD COLUMN domain_id   UUID NULL REFERENCES domains(id) ON DELETE SET NULL;
ALTER TABLE tenants ADD COLUMN environment TEXT NULL;
CREATE INDEX idx_tenants_domain ON tenants(domain_id);

CREATE TABLE audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_key_id  UUID NULL,
    action        TEXT NOT NULL,
    from_tenant_id UUID NULL,
    to_tenant_id   UUID NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_tenant;
DROP TABLE IF EXISTS audit_log;
DROP INDEX IF EXISTS idx_tenants_domain;
ALTER TABLE tenants DROP COLUMN IF EXISTS environment;
ALTER TABLE tenants DROP COLUMN IF EXISTS domain_id;
DROP TABLE IF EXISTS domains;
```

- [ ] **Step 2: Verify it parses + applies (dockertest path is exercised by later tasks)**

Run: `cd backend && gofmt -l . ; go build ./...`
Expected: build clean (SQL is applied by goose in integration tests; no Go change here). Sanity-check the SQL by eye: `+goose Up`/`+goose Down` markers present, Down reverses Up in opposite order.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/20260531120000_create_domains_and_audit.sql
git commit -m "feat(mt-1): migration — domains, tenants.domain_id/environment, audit_log"
```

---

## Task 2: tenant.Tenant gains DomainID + Environment; PGResolver selects them

AC#2 (membership needs domain_id), AC#3 (environment in listing).

**Files:**
- Modify: `backend/internal/tenant/tenant.go`
- Modify: `backend/internal/auth/resolver_pg.go`
- Modify: `backend/internal/auth/resolver_pg_test.go` (extend the existing `TestPGResolver_TenantByID`)

- [ ] **Step 1: Add fields to tenant.Tenant**

Edit `backend/internal/tenant/tenant.go` — add two fields to the struct:

```go
type Tenant struct {
	ID                uuid.UUID
	Name              string
	Plan              string
	RateLimitPerMin   int
	DataRetentionDays int
	CreatedAt         time.Time
	DomainID          uuid.UUID // uuid.Nil when the tenant is ungrouped (NULL domain_id)
	Environment       string    // "" when unset (NULL environment)
}
```

- [ ] **Step 2: Write the failing integration test (extend existing)**

Edit `backend/internal/auth/resolver_pg_test.go` — the existing `TestPGResolver_TenantByID` (build tag `integration`) creates a tenant and looks it up. Extend it so the tenant has a domain + environment and assert they round-trip:

```go
// inside TestPGResolver_TenantByID, after creating the plain tenant + before the existing asserts,
// add a domained tenant and assert DomainID/Environment are populated:
var domID string
require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO domains(name) VALUES('acme-corp') RETURNING id").Scan(&domID))
var dtID string
require.NoError(t, db.QueryRowContext(ctx,
	"INSERT INTO tenants(name, domain_id, environment) VALUES('shop-prod',$1,'prod') RETURNING id", domID).Scan(&dtID))

did, err := uuid.Parse(dtID)
require.NoError(t, err)
got2, err := r.TenantByID(ctx, did)
require.NoError(t, err)
assert.Equal(t, "shop-prod", got2.Name)
assert.Equal(t, "prod", got2.Environment)
assert.Equal(t, domID, got2.DomainID.String())
```

- [ ] **Step 3: Run it; expect FAIL** (DomainID/Environment are zero — the SELECT doesn't fetch them yet)

Run: `cd backend && go test -tags=integration -run TestPGResolver_TenantByID -timeout 240s ./internal/auth/`
Expected: FAIL (Environment == "", DomainID == Nil).

- [ ] **Step 4: Update both queries in resolver_pg.go**

In `ResolveBearer`, add `t.domain_id, t.environment` to the SELECT and scan them via nullable holders. In `TenantByID`, same. Use `uuid.NullUUID` + `sql.NullString` (google/uuid's `NullUUID` implements `sql.Scanner`):

```go
// ResolveBearer query: append to the tenant columns
query := `
	SELECT k.id, k.tenant_id, k.name, k.hashed_key, k.scope, k.revoked_at, k.last_used_at, k.created_at,
	       t.id, t.name, t.plan, t.rate_limit_per_min, t.data_retention_days, t.created_at,
	       t.domain_id, t.environment
	FROM api_keys k JOIN tenants t ON t.id = k.tenant_id
	WHERE k.revoked_at IS NULL
`
// ... in the row loop:
var dn uuid.NullUUID
var env sql.NullString
if err := rows.Scan(
	&k.ID, &k.TenantID, &k.Name, &k.HashedKey, &k.Scope,
	&k.RevokedAt, &k.LastUsedAt, &k.CreatedAt,
	&t.ID, &t.Name, &t.Plan, &t.RateLimitPerMin, &t.DataRetentionDays, &t.CreatedAt,
	&dn, &env,
); err != nil {
	return apikey.ApiKey{}, tenant.Tenant{}, err
}
if dn.Valid { t.DomainID = dn.UUID }
t.Environment = env.String
```

```go
// TenantByID:
query := `
	SELECT id, name, plan, rate_limit_per_min, data_retention_days, created_at, domain_id, environment
	FROM tenants
	WHERE id = $1
`
var t tenant.Tenant
var dn uuid.NullUUID
var env sql.NullString
err := p.db.QueryRowContext(ctx, query, id).Scan(
	&t.ID, &t.Name, &t.Plan, &t.RateLimitPerMin, &t.DataRetentionDays, &t.CreatedAt, &dn, &env,
)
if errors.Is(err, sql.ErrNoRows) {
	return tenant.Tenant{}, ErrTenantNotFound
}
if err != nil {
	return tenant.Tenant{}, err
}
if dn.Valid { t.DomainID = dn.UUID }
t.Environment = env.String
return t, nil
```

(`sql` is already imported in resolver_pg.go; `uuid` is already imported.)

- [ ] **Step 5: Run it; expect PASS.**

Run: `cd backend && go test -tags=integration -run TestPGResolver_TenantByID -timeout 240s ./internal/auth/`
Expected: PASS.

- [ ] **Step 6: Run the unit build (non-integration) to ensure nothing else broke**

Run: `cd backend && go build ./... && go test ./internal/auth/ ./internal/tenant/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/tenant/tenant.go backend/internal/auth/resolver_pg.go backend/internal/auth/resolver_pg_test.go
git commit -m "feat(mt-1): tenant DomainID/Environment + resolver selects them"
```

---

## Task 3: Auth middleware — `domain` scope branch + context scope/keyid

AC#2, AC#5 (isolation by construction).

**Files:**
- Modify: `backend/internal/auth/ctx.go`
- Modify: `backend/internal/auth/middleware.go`
- Modify: `backend/internal/auth/middleware_test.go` (extend the fake + add domain tests)

- [ ] **Step 1: Add context helpers for scope + key id**

Edit `backend/internal/auth/ctx.go` — add two context keys + four functions (mirror the existing tenant helpers). Append:

```go
// (add new unexported key constants alongside the existing tenantIDKey/tenantNameKey)
//   scopeKey
//   keyIDKey

// WithScope stores the resolved api key's scope on the context so identity
// handlers (e.g. GET /api/v1/tenants) can decide what the caller may enumerate.
func WithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, scopeKey, scope)
}

// Scope returns the api key scope set by the auth middleware ("" if absent).
func Scope(ctx context.Context) string {
	s, _ := ctx.Value(scopeKey).(string)
	return s
}

// WithKeyID stores the resolved api key id (the audit actor).
func WithKeyID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keyIDKey, id)
}

// KeyID returns the api key id set by the auth middleware (uuid.Nil if absent).
func KeyID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(keyIDKey).(uuid.UUID)
	return id
}
```

Read the existing `ctx.go` first to see the exact key-constant declaration style (an unexported `type ctxKey int` + `const (...)` block is typical) and add `scopeKey`, `keyIDKey` to it. Ensure `uuid` is imported (it is, for the tenant helpers).

- [ ] **Step 2: Write the failing middleware tests**

Edit `backend/internal/auth/middleware_test.go`. The `fakeResolver` already has `TenantByID`. Add a helper that builds a domain-scoped key whose tenant has a domain, plus an in-domain peer and an out-of-domain tenant, then add tests. Append:

```go
const ScopeDomainTest = "domain" // mirror auth.ScopeDomain for readability

// newDomainResolver builds: a `domain`-scoped key on homeTenant (domain D);
// peerTenant also in D; otherTenant in a different domain.
func newDomainResolver(t *testing.T) (f *fakeResolver, home, peer, other uuid.UUID, plain string) {
	t.Helper()
	domainD := uuid.New()
	domainOther := uuid.New()
	home, peer, other = uuid.New(), uuid.New(), uuid.New()
	plain = "plain-domain-key"
	hashed, err := apikey.Hash(plain)
	require.NoError(t, err)
	f = &fakeResolver{
		keys: map[string]apikey.ApiKey{
			hashed: {TenantID: home, Name: "dk", HashedKey: hashed, Scope: auth.ScopeDomain},
		},
		tenants: map[uuid.UUID]tenant.Tenant{
			home:  {ID: home, Name: "shop-prod", DomainID: domainD},
			peer:  {ID: peer, Name: "shop-staging", DomainID: domainD},
			other: {ID: other, Name: "evil", DomainID: domainOther},
		},
	}
	return f, home, peer, other, plain
}

func TestMiddleware_Domain_SwitchesWithinDomain(t *testing.T) {
	f, _, peer, _, plain := newDomainResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := auth.TenantID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, peer, got, "domain key must adopt an in-domain target")
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", peer.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_Domain_OutOfDomain_403(t *testing.T) {
	f, _, _, other, plain := newDomainResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not run for an out-of-domain target")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("X-Tenant-Id", other.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestMiddleware_Domain_NoHeader_PinsToOwn(t *testing.T) {
	f, home, _, _, plain := newDomainResolver(t)
	handler := auth.Middleware(f)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := auth.TenantID(r.Context())
		assert.Equal(t, home, got)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 3: Run; expect FAIL** (`auth.ScopeDomain` undefined; domain key currently ignores the header → the out-of-domain test would not 403 and the switch test would stay pinned to home).

Run: `cd backend && go test -run 'TestMiddleware_Domain' ./internal/auth/`
Expected: compile error (ScopeDomain undefined), then after defining it, behavioral failures.

- [ ] **Step 4: Implement the domain branch in middleware.go**

Add the constant + `tenantInDomain` helper, generalize the header branch to cover both scopes, and stash scope + key id on context.

```go
// add alongside ScopeServiceAI:
// ScopeDomain grants switching among tenants of the key's own domain via
// X-Tenant-Id, validated by a shared non-NULL domain_id (PLATFORM-MT-1 / ADR-0004).
const ScopeDomain = "domain"

// tenantInDomain reports whether target is in the same (non-NULL) domain as key.
func tenantInDomain(target, key tenant.Tenant) bool {
	return key.DomainID != uuid.Nil && target.DomainID == key.DomainID
}
```

Replace the existing `if k.Scope == ScopeServiceAI { ... }` block with:

```go
// service:ai → any tenant; domain → only within the key's own domain.
// Every other scope ignores the header and stays pinned to its own tenant.
if k.Scope == ScopeServiceAI || k.Scope == ScopeDomain {
	if raw := req.Header.Get(HeaderTenantID); raw != "" {
		target, err := resolveTargetTenant(req.Context(), r, raw)
		switch {
		case errors.Is(err, errBadTenantID):
			http.Error(w, "invalid X-Tenant-Id", http.StatusBadRequest)
			return
		case errors.Is(err, ErrTenantNotFound):
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if k.Scope == ScopeDomain && !tenantInDomain(target, tn) {
			http.Error(w, "tenant not in your domain", http.StatusForbidden)
			return
		}
		tn = target
	}
}

ctx := WithTenant(req.Context(), tn.ID, tn.Name)
ctx = WithScope(ctx, k.Scope)
ctx = WithKeyID(ctx, k.ID)
next.ServeHTTP(w, req.WithContext(ctx))
```

(Delete the old `ctx := WithTenant(...)` + `next.ServeHTTP` lines that followed the old block — they're replaced above. `tenant` and `uuid` are already imported.)

- [ ] **Step 5: Run the domain tests + the full auth unit suite; expect PASS**

Run: `cd backend && go test ./internal/auth/`
Expected: PASS (existing service:ai + new domain tests). The existing `TestMiddleware_ServiceAI_*` and `TestMiddleware_NonServiceAI_IgnoresTenantHeader` must remain green — service:ai still reaches any tenant, read-write still ignores the header.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/auth/ctx.go backend/internal/auth/middleware.go backend/internal/auth/middleware_test.go
git commit -m "feat(mt-1): domain scope branch (in-domain switch / out-of-domain 403) + ctx scope/keyid"
```

---

## Task 4: `GET /api/v1/tenants` + `POST /api/v1/tenants/switch`

AC#3, AC#6 (audit write).

**Files:**
- Create: `backend/internal/identity/tenants_repo.go`
- Create: `backend/internal/identity/tenants_handler.go`
- Create: `backend/internal/identity/tenants_handler_test.go`
- Create: `backend/internal/identity/tenants_repo_test.go` (integration)
- Modify: `backend/internal/httpsrv/server.go`
- Modify: `backend/cmd/gateway/main.go`

- [ ] **Step 1: Repo — domain peers, single, audit insert**

Create `backend/internal/identity/tenants_repo.go`:

```go
package identity

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// TenantView is the public shape returned to the topbar selector.
type TenantView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// VisibleTenants returns the tenants the caller may enumerate. For a domain-scoped
// caller whose tenant has a non-NULL domain_id, that is every tenant in the domain;
// otherwise just the caller's own tenant. `scope` is the api-key scope; `tid` is the
// caller's current context tenant (home or in-domain peer — same domain either way).
func (r *Repo) VisibleTenants(ctx context.Context, tid uuid.UUID, scope string) ([]TenantView, error) {
	if scope == "domain" {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, name, COALESCE(environment, '')
			FROM tenants
			WHERE domain_id IS NOT NULL
			  AND domain_id = (SELECT domain_id FROM tenants WHERE id = $1)
			ORDER BY environment, name
		`, tid)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TenantView
		for rows.Next() {
			var v TenantView
			if err := rows.Scan(&v.ID, &v.Name, &v.Environment); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if len(out) > 0 {
			return out, rows.Err()
		}
		// domain key whose own tenant has NULL domain_id → fall through to single.
	}
	return r.singleTenant(ctx, tid)
}

func (r *Repo) singleTenant(ctx context.Context, tid uuid.UUID) ([]TenantView, error) {
	var v TenantView
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(environment, '') FROM tenants WHERE id = $1`, tid,
	).Scan(&v.ID, &v.Name, &v.Environment)
	if err != nil {
		return nil, err
	}
	return []TenantView{v}, nil
}

// DomainID returns a tenant's domain_id (uuid.Nil if NULL/absent).
func (r *Repo) DomainID(ctx context.Context, tid uuid.UUID) (uuid.UUID, error) {
	var dn uuid.NullUUID
	err := r.db.QueryRowContext(ctx, `SELECT domain_id FROM tenants WHERE id = $1`, tid).Scan(&dn)
	if err != nil {
		return uuid.Nil, err
	}
	if dn.Valid {
		return dn.UUID, nil
	}
	return uuid.Nil, nil
}

// InsertSwitchAudit records one tenant-switch action.
func (r *Repo) InsertSwitchAudit(ctx context.Context, fromTenant, toTenant, actorKey uuid.UUID) error {
	var actor any
	if actorKey != uuid.Nil {
		actor = actorKey
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_log (tenant_id, actor_key_id, action, from_tenant_id, to_tenant_id)
		VALUES ($1, $2, 'tenant_switch', $3, $4)
	`, fromTenant, actor, fromTenant, toTenant)
	return err
}
```

- [ ] **Step 2: Handler — seam interface + Create/List**

Create `backend/internal/identity/tenants_handler.go`:

```go
package identity

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// store is the repo seam the handler depends on (real = *Repo; tests = fake).
type store interface {
	VisibleTenants(ctx context.Context, tid uuid.UUID, scope string) ([]TenantView, error)
	DomainID(ctx context.Context, tid uuid.UUID) (uuid.UUID, error)
	InsertSwitchAudit(ctx context.Context, fromTenant, toTenant, actorKey uuid.UUID) error
}

type Handler struct{ s store }

func NewHandler(s store) *Handler { return &Handler{s: s} }

// List handles GET /api/v1/tenants.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tid, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	views, err := h.s.VisibleTenants(r.Context(), tid, auth.Scope(r.Context()))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

type switchReq struct {
	TenantID string `json:"tenant_id"`
}

// Switch handles POST /api/v1/tenants/switch — validate domain membership, audit once.
func (h *Handler) Switch(w http.ResponseWriter, r *http.Request) {
	from, err := auth.TenantID(r.Context())
	if err != nil {
		http.Error(w, "no tenant", http.StatusInternalServerError)
		return
	}
	var req switchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TenantID == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	to, err := uuid.Parse(req.TenantID)
	if err != nil {
		http.Error(w, "bad tenant_id", http.StatusBadRequest)
		return
	}
	// Only domain-scoped callers may switch; membership = shared non-NULL domain_id.
	if auth.Scope(r.Context()) != auth.ScopeDomain {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fromDom, err := h.s.DomainID(r.Context(), from)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	toDom, err := h.s.DomainID(r.Context(), to)
	if err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}
	if fromDom == uuid.Nil || toDom != fromDom {
		http.Error(w, "tenant not in your domain", http.StatusForbidden)
		return
	}
	if err := h.s.InsertSwitchAudit(r.Context(), from, to, auth.KeyID(r.Context())); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tenant_id": to.String()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 3: Failing handler unit tests (fake store)**

Create `backend/internal/identity/tenants_handler_test.go`:

```go
package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

type fakeStore struct {
	views   []TenantView
	domains map[uuid.UUID]uuid.UUID // tenant -> domain (absent = Nil)
	audits  int
}

func (f *fakeStore) VisibleTenants(_ context.Context, _ uuid.UUID, _ string) ([]TenantView, error) {
	return f.views, nil
}
func (f *fakeStore) DomainID(_ context.Context, tid uuid.UUID) (uuid.UUID, error) {
	return f.domains[tid], nil
}
func (f *fakeStore) InsertSwitchAudit(_ context.Context, _, _, _ uuid.UUID) error {
	f.audits++
	return nil
}

// ctxWith builds a request context as the middleware would (tenant + scope + key id).
func ctxWith(tid uuid.UUID, scope string) context.Context {
	ctx := auth.WithTenant(context.Background(), tid, "t")
	ctx = auth.WithScope(ctx, scope)
	ctx = auth.WithKeyID(ctx, uuid.New())
	return ctx
}

func TestList_ReturnsViews(t *testing.T) {
	home := uuid.New()
	fs := &fakeStore{views: []TenantView{{ID: home.String(), Name: "shop-prod", Environment: "prod"}}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil).WithContext(ctxWith(home, "domain"))
	h.List(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got []TenantView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "shop-prod", got[0].Name)
}

func TestSwitch_InDomain_200_Audits(t *testing.T) {
	home, peer, dom := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: dom, peer: dom}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + peer.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "domain"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, fs.audits)
}

func TestSwitch_OutOfDomain_403_NoAudit(t *testing.T) {
	home, other, dom, otherDom := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: dom, other: otherDom}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + other.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "domain"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, 0, fs.audits)
}

func TestSwitch_NonDomainScope_403(t *testing.T) {
	home, peer := uuid.New(), uuid.New()
	fs := &fakeStore{domains: map[uuid.UUID]uuid.UUID{home: uuid.New(), peer: uuid.New()}}
	h := NewHandler(fs)
	rec := httptest.NewRecorder()
	body := `{"tenant_id":"` + peer.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/switch", strings.NewReader(body)).WithContext(ctxWith(home, "read-write"))
	h.Switch(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 4: Run handler tests; expect FAIL → then PASS once steps 1–2 compile**

Run: `cd backend && go test ./internal/identity/`
Expected: after the repo+handler are in place, PASS (4 tests). (If you wrote tests first against missing files, you'll see compile errors → add the files → green.)

- [ ] **Step 5: Mount routes — extend httpsrv.NewRouter(resolver, db)**

Edit `backend/internal/httpsrv/server.go`:

```go
import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/identity"
)

func NewRouter(resolver auth.Resolver, db *sql.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID, chimiddleware.RealIP, chimiddleware.Recoverer)
	r.Use(chimiddleware.Logger)

	r.With(auth.Middleware(resolver)).Get("/healthz", Healthz)

	ih := identity.NewHandler(identity.NewRepo(db))
	r.Group(func(g chi.Router) {
		g.Use(auth.Middleware(resolver))
		g.Get("/api/v1/tenants", ih.List)
		g.Post("/api/v1/tenants/switch", ih.Switch)
	})

	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return r
}
```

- [ ] **Step 6: Pass db in gateway**

Edit `backend/cmd/gateway/main.go` line 42 — `router := httpsrv.NewRouter(resolver)` → `router := httpsrv.NewRouter(resolver, db)`.

- [ ] **Step 7: Integration test for the repo (dockertest)**

Create `backend/internal/identity/tenants_repo_test.go` with build tag `integration`, following the same dockertest TestMain pattern used in `backend/internal/auth/resolver_pg_test.go` (copy its `TestMain` verbatim: pool → postgres:16-alpine → goose.Up "../../migrations" → `var pgDSN string`). Then:

```go
//go:build integration

package identity_test

// ... (TestMain identical to resolver_pg_test.go, exposing pgDSN)

func TestRepo_VisibleTenants_DomainPeersVsSingle(t *testing.T) {
	ctx := context.Background()
	db, _ := sql.Open("pgx", pgDSN)
	defer db.Close()
	_, _ = db.ExecContext(ctx, "TRUNCATE audit_log, api_keys, tenants, domains RESTART IDENTITY CASCADE")

	var dom string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO domains(name) VALUES('d') RETURNING id").Scan(&dom))
	var prod, stg, lone string
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name,domain_id,environment) VALUES('prod',$1,'prod') RETURNING id", dom).Scan(&prod))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name,domain_id,environment) VALUES('stg',$1,'staging') RETURNING id", dom).Scan(&stg))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO tenants(name) VALUES('lone') RETURNING id").Scan(&lone))

	repo := identity.NewRepo(db)
	prodID := uuid.MustParse(prod)
	peers, err := repo.VisibleTenants(ctx, prodID, "domain")
	require.NoError(t, err)
	assert.Len(t, peers, 2) // prod + stg

	loneID := uuid.MustParse(lone)
	single, err := repo.VisibleTenants(ctx, loneID, "read-write")
	require.NoError(t, err)
	assert.Len(t, single, 1)

	// audit insert round-trips
	require.NoError(t, repo.InsertSwitchAudit(ctx, prodID, uuid.MustParse(stg), uuid.New()))
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_log WHERE action='tenant_switch'").Scan(&n))
	assert.Equal(t, 1, n)
}
```

- [ ] **Step 8: Verify**

Run: `cd backend && go build ./... && go test ./internal/identity/ && go test -tags=integration -run 'TestRepo_VisibleTenants' -timeout 240s ./internal/identity/`
Expected: unit (4) + integration (1) PASS; build clean (gateway now compiles with the db arg).

- [ ] **Step 9: Commit**

```bash
git add backend/internal/identity/ backend/internal/httpsrv/server.go backend/cmd/gateway/main.go
git commit -m "feat(mt-1): GET /api/v1/tenants + POST /api/v1/tenants/switch (audit) on gateway"
```

---

## Task 5: Frontend — tenants API + auth store + X-Tenant-Id interceptor

AC#4 (store half).

**Files:**
- Create: `frontend/src/api/tenants.ts`
- Modify: `frontend/src/stores/auth.ts`
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/stores/__tests__/auth.spec.ts` (if absent) or extend existing

- [ ] **Step 1: tenants API module**

Create `frontend/src/api/tenants.ts`:

```ts
import client from './client'

export interface TenantOption {
  id: string
  name: string
  environment: string
}

export async function fetchTenants(): Promise<TenantOption[]> {
  const { data } = await client.get('/api/v1/tenants')
  return (data ?? []) as TenantOption[]
}

export async function switchTenant(tenantId: string): Promise<void> {
  await client.post('/api/v1/tenants/switch', { tenant_id: tenantId })
}
```

- [ ] **Step 2: X-Tenant-Id interceptor**

Edit `frontend/src/api/client.ts` — extend the request interceptor to attach the active tenant header when set:

```ts
client.interceptors.request.use((cfg) => {
  const key = localStorage.getItem('apiKey')
  if (key) cfg.headers.Authorization = `Bearer ${key}`
  const active = localStorage.getItem('activeTenantId')
  if (active) cfg.headers['X-Tenant-Id'] = active
  return cfg
})
```

- [ ] **Step 3: Write the failing store test**

Create/extend `frontend/src/stores/__tests__/auth.spec.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../auth'

vi.mock('../../api/client', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))
vi.mock('../../api/tenants', () => ({
  fetchTenants: vi.fn().mockResolvedValue([
    { id: 'home', name: 'shop-prod', environment: 'prod' },
    { id: 'peer', name: 'shop-staging', environment: 'staging' },
  ]),
  switchTenant: vi.fn().mockResolvedValue(undefined),
}))

describe('auth store — tenant switching', () => {
  beforeEach(() => { setActivePinia(createPinia()); localStorage.clear() })

  it('switchActiveTenant sets active id + persists + sends header next request', async () => {
    const auth = useAuthStore()
    auth.tenantId = 'home'
    await auth.switchActiveTenant('peer')
    expect(auth.activeTenantId).toBe('peer')
    expect(localStorage.getItem('activeTenantId')).toBe('peer')
  })

  it('logout clears the active tenant', () => {
    const auth = useAuthStore()
    localStorage.setItem('activeTenantId', 'peer')
    auth.activeTenantId = 'peer'
    auth.logout()
    expect(auth.activeTenantId).toBeNull()
    expect(localStorage.getItem('activeTenantId')).toBeNull()
  })
})
```

- [ ] **Step 4: Run; expect FAIL** (`switchActiveTenant`/`activeTenantId` undefined).

Run: `cd frontend && npx vitest run src/stores/__tests__/auth.spec.ts`

- [ ] **Step 5: Extend the auth store**

Edit `frontend/src/stores/auth.ts`:

```ts
import { defineStore } from 'pinia'
import client from '../api/client'
import { fetchTenants, switchTenant, type TenantOption } from '../api/tenants'

interface State {
  tenantId: string | null
  tenantName: string | null
  activeTenantId: string | null
  domainTenants: TenantOption[]
}

export const useAuthStore = defineStore('auth', {
  state: (): State => ({
    tenantId: null,
    tenantName: null,
    activeTenantId: null,
    domainTenants: [],
  }),
  getters: {
    isAuthenticated: (s) => s.tenantId !== null,
    activeTenant: (s) =>
      s.domainTenants.find((t) => t.id === s.activeTenantId) ?? null,
  },
  actions: {
    async loadTenants() {
      try {
        this.domainTenants = await fetchTenants()
        // default the active selection to the persisted one (if still a member) else home.
        const persisted = localStorage.getItem('activeTenantId')
        const valid = persisted && this.domainTenants.some((t) => t.id === persisted)
        this.activeTenantId = valid ? persisted : this.tenantId
        this.persistActive()
      } catch {
        this.domainTenants = this.tenantId
          ? [{ id: this.tenantId, name: this.tenantName ?? '', environment: '' }]
          : []
        this.activeTenantId = this.tenantId
      }
    },
    persistActive() {
      if (this.activeTenantId) localStorage.setItem('activeTenantId', this.activeTenantId)
      else localStorage.removeItem('activeTenantId')
    },
    async switchActiveTenant(tenantId: string) {
      await switchTenant(tenantId) // 403 throws → caller toasts, selection unchanged
      this.activeTenantId = tenantId
      this.persistActive()
    },
    async login(apiKey: string) {
      localStorage.setItem('apiKey', apiKey)
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
        await this.loadTenants()
      } catch (e) {
        localStorage.removeItem('apiKey')
        this.tenantId = null
        this.tenantName = null
        throw e
      }
    },
    logout() {
      localStorage.removeItem('apiKey')
      localStorage.removeItem('activeTenantId')
      this.tenantId = null
      this.tenantName = null
      this.activeTenantId = null
      this.domainTenants = []
    },
    async restore() {
      const key = localStorage.getItem('apiKey')
      if (!key) return
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
        await this.loadTenants()
      } catch {
        this.logout()
      }
    },
  },
})
```

- [ ] **Step 6: Run; expect PASS + full vitest + build**

Run: `cd frontend && npx vitest run src/stores/__tests__/auth.spec.ts && npm run build && npx vitest run`
Expected: store tests PASS; build clean; full suite green (existing login tests still pass — `login` now also calls `loadTenants`, which is mocked in store tests; for any existing LoginView/auth test that hits `/healthz`, ensure the tenants call is mocked or tolerated. If an existing test breaks because `fetchTenants` isn't mocked there, add the mock to that test — do not change behavior).

- [ ] **Step 7: Commit**

```bash
git add frontend/src/api/tenants.ts frontend/src/api/client.ts frontend/src/stores/auth.ts frontend/src/stores/__tests__/auth.spec.ts
git commit -m "feat(mt-1): tenants api + auth store active-tenant switching + X-Tenant-Id interceptor"
```

---

## Task 6: ScopePill — live Project switcher + Env grouping

AC#4 (UI half). Replaces the PLATFORM-UI-1 static stub. Keeps testids `scope-domain`/`scope-project`/`scope-env`.

**Files:**
- Modify: `frontend/src/components/ScopePill.vue`
- Modify: `frontend/src/components/__tests__/ScopePill.spec.ts`

- [ ] **Step 1: Update the failing test**

Edit `frontend/src/components/__tests__/ScopePill.spec.ts` — keep the existing two tests (project shows tenant; domain+env exist) and add a switch test:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ScopePill from '../ScopePill.vue'
import { useAuthStore } from '../../stores/auth'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })

describe('ScopePill', () => {
  beforeEach(() => { setActivePinia(createPinia()); localStorage.clear() })

  it('shows the current active tenant as the Project segment', () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    auth.tenantId = 'home'
    auth.activeTenantId = 'home'
    auth.domainTenants = [{ id: 'home', name: 'acme', environment: 'prod' }]
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.get('[data-testid="scope-project"]').text()).toContain('acme')
  })

  it('renders Domain and Env segments', () => {
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.find('[data-testid="scope-domain"]').exists()).toBe(true)
    expect(w.find('[data-testid="scope-env"]').exists()).toBe(true)
  })

  it('clicking a domain peer calls switchActiveTenant', async () => {
    const auth = useAuthStore()
    auth.tenantId = 'home'
    auth.tenantName = 'shop-prod'
    auth.activeTenantId = 'home'
    auth.domainTenants = [
      { id: 'home', name: 'shop-prod', environment: 'prod' },
      { id: 'peer', name: 'shop-staging', environment: 'staging' },
    ]
    const spy = vi.spyOn(auth, 'switchActiveTenant').mockResolvedValue()
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    await w.get('[data-testid="scope-project"]').trigger('click')
    await w.get('[data-testid="tenant-opt-peer"]').trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenCalledWith('peer')
  })
})
```

- [ ] **Step 2: Run; expect FAIL** (no dropdown / `tenant-opt-*`).

- [ ] **Step 3: Rewrite ScopePill.vue**

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const auth = useAuthStore()

const open = ref(false)
const errorMsg = ref('')

const activeName = computed(() => {
  const a = auth.domainTenants.find((x) => x.id === auth.activeTenantId)
  return a?.name ?? auth.tenantName ?? '—'
})
const activeEnv = computed(() => {
  const a = auth.domainTenants.find((x) => x.id === auth.activeTenantId)
  return a?.environment || '—'
})
const canSwitch = computed(() => auth.domainTenants.length > 1)

async function pick(id: string) {
  open.value = false
  if (id === auth.activeTenantId) return
  try {
    await auth.switchActiveTenant(id)
    errorMsg.value = ''
    // re-query the current view under the new tenant
    window.location.reload()
  } catch {
    errorMsg.value = t('shell.switchDenied')
  }
}
</script>

<template>
  <div class="scope-pill">
    <div class="sp-seg" data-testid="scope-domain" :title="t('shell.domainReadonly')">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>
      <span class="k" style="color: var(--text-3);">Domain</span><span>{{ t('shell.defaultDomain') }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="dd" :class="{ open }" style="display:inline-block;">
      <div
        class="sp-seg" data-testid="scope-project"
        :style="{ cursor: canSwitch ? 'pointer' : 'default' }"
        :title="t('shell.projectIsTenant')"
        @click.stop="canSwitch && (open = !open)"
      >
        <span class="k" style="color: var(--text-3);">Project</span>
        <span style="color: var(--accent);">●</span><span>{{ activeName }}</span>
      </div>
      <div v-if="canSwitch" class="dd-menu" style="min-width: 220px;">
        <div class="dd-section">{{ t('shell.switchTenant') }}</div>
        <div
          v-for="opt in auth.domainTenants" :key="opt.id"
          class="dd-item" :class="{ selected: opt.id === auth.activeTenantId }"
          :data-testid="`tenant-opt-${opt.id}`"
          @click="pick(opt.id)"
        >
          <span>{{ opt.name }}</span>
          <span class="kbd">{{ opt.environment || '—' }}</span>
        </div>
      </div>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-env" :title="t('shell.envReadonly')">
      <span class="dot" style="width:6px;height:6px;border-radius:50%;background:var(--success);" />
      <span>{{ activeEnv }}</span>
    </div>
  </div>
</template>

<style scoped>
.sp-sep { color: var(--text-3); padding: 0 2px; }
.sp-seg .k { margin-right: 2px; }
</style>
```

Add the new i18n keys used here to **both** locale files' `shell` block (T7 seeds them too, but add now): `switchTenant` ('切换租户' / 'Switch tenant'), `switchDenied` ('无权切换到该租户' / 'Not allowed to switch to that tenant'). (vue-i18n echoes missing keys, so tests pass without them; add for real UX.)

- [ ] **Step 4: Run ScopePill tests + full suite + build**

Run: `cd frontend && npx vitest run src/components/__tests__/ScopePill.spec.ts && npm run build && npx vitest run`
Expected: ScopePill 3 PASS; build clean; full suite green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ScopePill.vue frontend/src/components/__tests__/ScopePill.spec.ts frontend/src/i18n/locales/zh-CN.ts frontend/src/i18n/locales/en-US.ts
git commit -m "feat(mt-1): ScopePill live tenant switcher (project dropdown + env)"
```

---

## Task 7: Seed + reverse E2E + full verification gate

AC#5, AC#7.

**Files:**
- Modify: `deploy/seed.sql`
- Create: `frontend/e2e/tenant-switch.spec.ts`

- [ ] **Step 1: Seed a demo domain + env-tagged tenants + a domain key**

Edit `deploy/seed.sql`. After the existing tenants/keys inserts, add:

```sql
-- PLATFORM-MT-1: a demo domain with two env-tagged tenants + a domain-scoped key.
INSERT INTO domains(id, name) VALUES
  ('d0000000-0000-0000-0000-000000000001', 'demo-domain');

INSERT INTO tenants(id, name, plan, domain_id, environment) VALUES
  ('33333333-3333-3333-3333-333333333333', 'shop-prod',    'free', 'd0000000-0000-0000-0000-000000000001', 'prod'),
  ('44444444-4444-4444-4444-444444444444', 'shop-staging', 'free', 'd0000000-0000-0000-0000-000000000001', 'staging');

-- domain-scoped key (plaintext test-key-domain, dev-only): may switch among demo-domain tenants.
-- Hash generated via `go run ./backend/cmd/seed-hash test-key-domain` (bcrypt cost 10) — REGENERATE before committing.
INSERT INTO api_keys(tenant_id, name, hashed_key, scope) VALUES
  ('33333333-3333-3333-3333-333333333333', 'demo-domain-key', '$2a$10$REPLACE_WITH_REAL_HASH', 'domain');
```

Generate the real hash and replace the placeholder:

```bash
cd backend && go run ./cmd/seed-hash test-key-domain   # if cmd/seed-hash exists; else use the existing hash tool
```

Update the seed header comment to note `test-key-domain` is a dev-only plaintext. Also add `audit_log` and `domains` to the leading `TRUNCATE` so re-seed is clean:
`TRUNCATE audit_log, api_keys, tenants, domains, metering_events RESTART IDENTITY CASCADE;` (annotations is already truncated via CASCADE on tenants; keep existing TRUNCATE targets + add audit_log, domains).

- [ ] **Step 2: Backend integration — domain switch isolation (extends auth integration)**

Add to `backend/internal/auth/resolver_pg_test.go` (integration) a test that a `domain` key resolves and a peer/out-of-domain are distinguishable via TenantByID + tenantInDomain is covered by the unit tests in T3. (The middleware behavior is already unit-tested in T3; the integration layer just confirms the PG round-trip of domain_id, done in T2.) No new file needed — T2 + T3 already cover the isolation logic. **This step is a checkpoint: confirm `go test -tags=integration ./internal/auth/ ./internal/identity/` is green.**

Run: `cd backend && go test -tags=integration -timeout 300s ./internal/auth/ ./internal/identity/`
Expected: PASS.

- [ ] **Step 3: Frontend e2e — switch via ScopePill**

Create `frontend/e2e/tenant-switch.spec.ts`:

```ts
import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('domain key lists peers and switches active tenant', async ({ page }) => {
  await login(page, 'test-key-domain')
  // ScopePill Project segment is clickable (more than one tenant)
  await page.getByTestId('scope-project').click()
  await expect(page.getByTestId('tenant-opt-44444444-4444-4444-4444-444444444444')).toBeVisible()
  await page.getByTestId('tenant-opt-44444444-4444-4444-4444-444444444444').click()
  // after switch the page reloads; the active env reflects staging
  await expect(page.getByTestId('scope-env')).toContainText('staging')
})

test('read-write key shows a single read-only project (no peers)', async ({ page }) => {
  await login(page, 'test-key-acme')
  // acme has no domain → single tenant → clicking does not open a peer list
  await page.getByTestId('scope-project').click()
  await expect(page.getByTestId('scope-project')).toContainText('acme')
})

test('out-of-domain switch is blocked by the backend (403)', async ({ request }) => {
  const res = await request.post('/api/v1/tenants/switch', {
    headers: { Authorization: 'Bearer test-key-domain', 'Content-Type': 'application/json' },
    data: { tenant_id: '11111111-1111-1111-1111-111111111111' }, // acme, outside demo-domain
  })
  expect(res.status()).toBe(403)
})
```

- [ ] **Step 4: Rebuild images (LESSON: `make up` does not --build), seed, run e2e**

```bash
cd /Users/huangbaixun/code_space/openaiops-platform
make migrate-up                      # apply the new PG migration (or `make up` re-runs migrate)
docker-compose -f deploy/docker-compose.yml build gateway frontend
docker-compose -f deploy/docker-compose.yml up -d --no-deps gateway frontend
make seed
cd frontend && npx playwright test tenant-switch.spec.ts
```
Expected: 3 passed. (gateway rebuilt because its router signature changed; frontend rebuilt for the ScopePill/store changes.)

- [ ] **Step 5: FULL regression gate**

```bash
cd backend && go test ./... && go test -tags=integration -timeout 300s ./internal/auth/ ./internal/identity/ ./internal/query/
cd frontend && npx vitest run && npm run build && npx playwright test
```
Expected: all backend unit + integration green; frontend vitest + build + the **full** Playwright suite (existing 31 shell/feature specs + the 3 new switch specs) green. The existing cross-tenant isolation e2e (acme writes / beta reads → 0) must remain green — proving MT-1 did not weaken isolation.

- [ ] **Step 6: Commit**

```bash
git add deploy/seed.sql frontend/e2e/tenant-switch.spec.ts
git commit -m "test(mt-1): seed demo domain + e2e (switch, single read-only, out-of-domain 403)"
```

- [ ] **Step 7: Hand off to verification-before-completion → finishing-a-development-branch.**

---

## Self-review notes

- **Spec coverage:** migration→T1; domain scope branch→T3; tenants endpoints + audit→T4; store/interceptor→T5; ScopePill→T6; isolation reverse-test + seed→T7; ADR-0004 already written. All 7 ACs traced (table above).
- **Placeholder scan:** the seed hash is the one intentional `REPLACE_WITH_REAL_HASH` — T7 Step 1 instructs generating it before commit (not a plan placeholder, an explicit action). No "TBD"/"handle errors" elsewhere; every code step shows code.
- **Type consistency:** `tenant.Tenant.DomainID uuid.UUID` (Nil = NULL) + `.Environment string` used identically in T2/T3/T4. `auth.ScopeDomain="domain"`, `auth.Scope(ctx)`, `auth.KeyID(ctx)`, `tenantInDomain(target,key)` consistent T3↔T4. `identity.TenantView{ID,Name,Environment}` ↔ frontend `TenantOption{id,name,environment}` (JSON tags match). Store `activeTenantId`/`domainTenants`/`switchActiveTenant`/`loadTenants` consistent T5↔T6.
- **Out-of-scope respected:** no users/sessions; env is metadata only (no access gate); no domain CRUD UI; `domain` key confined to its domain (cross-domain → 403). Data layer (`MustTenantScope`/Row Policy) untouched.
