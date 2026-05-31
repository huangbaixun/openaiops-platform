# PLATFORM-ANN-1 — Annotations Hardening (close D8 + D9) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use harness:subagent-driven-development (recommended) or harness:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/specs/2026-05-31-platform-ann-1-annotations-hardening-design.md` (feature `PLATFORM-ANN-1`). No ADR.

**Goal:** Close two ASK-2 drift items — extend the annotations e2e to cover the badge on trace-detail + topology nodes (D8), and add a background pruner in `cmd/query` that nulls idempotency keys older than a retention window so the partial unique index stays bounded (D9).

**Architecture:** A maintenance repo method `AnnotationsRepo.PruneIdempotencyKeys(ctx, days)` runs a single `UPDATE … SET idempotency_key = NULL` (tenant-unscoped retention; clears keys only, preserves content). A tiny `AnnotationsPruner` goroutine in `cmd/query` calls it on boot + on a ticker, configured by two env vars with defaults. D8 is pure Playwright additions against the live seeded stack.

**Tech Stack:** Go 1.25 (database/sql + pgx, log/slog), dockertest (PG fixture), Playwright.

---

## Acceptance-criteria → task traceability

| AC | Criterion (abridged) | Task(s) |
|---|---|---|
| 1 | e2e asserts badge on trace-detail + graph-node-ann-checkout on /topology (+ existing cases) | T3 |
| 2 | PruneIdempotencyKeys nulls old key, returns count, preserves content; integration proves null/keep/dedupe-lapse | T1 |
| 3 | cmd/query background pruner: boot + every interval (default 24h), retention default 30d, logs, retries, stops on shutdown | T2 |
| 4 | Config defaults unit-tested; all existing tests + full Playwright green | T2 (config test), T3 (gate) |
| 5 | known_drift D8 + D9 closed | T3 |

No orphan criteria. No task touches `out_of_scope` (no per-tenant retention, no annotation deletion, no schema change, no ADR).

---

## File structure

**Modified:**
- `backend/internal/query/annotations_repo.go` — add `PruneIdempotencyKeys`.
- `backend/internal/config/config.go` — two env vars + defaults.
- `backend/cmd/query/main.go` — start the pruner goroutine, cancel on shutdown.
- `frontend/e2e/annotations.spec.ts` — two new assertions.

**New:**
- `backend/internal/query/annotations_pruner.go` — `AnnotationsPruner` goroutine.
- tests: extend `backend/internal/query/annotations_repo_test.go` (integration); add a config unit test (extend `backend/internal/config/config_test.go` if present, else create); add a pruner integration test (in `annotations_repo_test.go` or a new `annotations_pruner_test.go`).

**Untouched:** annotations table DDL, handler, frontend app code.

---

## Task 1: `PruneIdempotencyKeys` repo method

AC#2.

**Files:**
- Modify: `backend/internal/query/annotations_repo.go`
- Modify: `backend/internal/query/annotations_repo_test.go` (integration)

- [ ] **Step 1: Write the failing integration test**

Append to `backend/internal/query/annotations_repo_test.go` (it's `package query_test`, build tag `integration`; uses `pgForAnnotations(t) (*sql.DB, t1, t2 string)` which migrates + truncates + seeds tenants acme/beta). Read the existing `AnnotationInput` type + `Insert` signature (`Insert(ctx, tenantID uuid.UUID, in AnnotationInput, idemKey string) (uuid.UUID, bool, error)` → returns (id, created, err)) before writing, and match field names.

```go
func TestAnnotationsRepo_PruneIdempotencyKeys(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)

	tid := uuid.MustParse(t1)
	// Two annotations with idempotency keys: one stamped 40 days ago, one now.
	// created_at must be set explicitly (repo.Insert defaults it to now()), so insert raw.
	_, err := db.ExecContext(ctx, `
		INSERT INTO annotations (tenant_id, target_type, target_id, kind, payload, ts, idempotency_key, created_at)
		VALUES ($1,'service','checkout','ai_rca','{}'::jsonb, now(), 'old-key',  now() - interval '40 days'),
		       ($1,'service','payment', 'ai_rca','{}'::jsonb, now(), 'recent-key', now())
	`, tid)
	require.NoError(t, err)

	n, err := repo.PruneIdempotencyKeys(ctx, 30)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "only the 40-day-old key is pruned")

	var oldKey, recentKey *string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT idempotency_key FROM annotations WHERE target_id='checkout'`).Scan(&oldKey))
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT idempotency_key FROM annotations WHERE target_id='payment'`).Scan(&recentKey))
	assert.Nil(t, oldKey, "old key nulled")
	require.NotNil(t, recentKey)
	assert.Equal(t, "recent-key", *recentKey, "recent key kept")

	// content preserved: both annotations still exist
	var cnt int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM annotations WHERE tenant_id=$1`, tid).Scan(&cnt))
	assert.Equal(t, 2, cnt)

	// dedupe lapses for the freed key: a fresh insert reusing 'old-key' creates a NEW row.
	_, createdOld, err := repo.Insert(ctx, tid,
		query.AnnotationInput{TargetType: "service", TargetID: "checkout", Kind: "ai_rca", Payload: []byte("{}"), TS: time.Now()},
		"old-key")
	require.NoError(t, err)
	assert.True(t, createdOld, "freed key is reusable -> new row created")

	// recent key still dedupes to its existing row.
	_, createdRecent, err := repo.Insert(ctx, tid,
		query.AnnotationInput{TargetType: "service", TargetID: "payment", Kind: "ai_rca", Payload: []byte("{}"), TS: time.Now()},
		"recent-key")
	require.NoError(t, err)
	assert.False(t, createdRecent, "live key still dedupes -> no new row")
}
```
IMPORTANT: read `AnnotationInput`'s real field names/types in `annotations_repo.go` and fix the struct literal above to match (the exact field set may differ — Payload may be `json.RawMessage` or `[]byte`; TS may be `time.Time`). Keep the assertion intent identical.

- [ ] **Step 2: Run it; expect FAIL** (`PruneIdempotencyKeys` undefined)

Run: `cd backend && go test -tags=integration -timeout 240s -run TestAnnotationsRepo_PruneIdempotencyKeys ./internal/query/`
Expected: compile error (method missing). Docker required; if down → BLOCKED.

- [ ] **Step 3: Implement `PruneIdempotencyKeys`**

Add to `annotations_repo.go` (the file already imports `context`, `database/sql`, `fmt`):

```go
// PruneIdempotencyKeys nulls idempotency_key for annotations whose created_at is older
// than `days` days, returning the number of rows affected. Maintenance op — intentionally
// tenant-UNSCOPED (retention runs across all tenants); it only clears keys, never reads or
// exposes annotation content. Nulling frees the row from the partial unique index
// uq_annotations_idem (WHERE idempotency_key IS NOT NULL), bounding it. PLATFORM-ANN-1 / D9.
func (r *AnnotationsRepo) PruneIdempotencyKeys(ctx context.Context, days int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE annotations
		   SET idempotency_key = NULL
		 WHERE idempotency_key IS NOT NULL
		   AND created_at < now() - make_interval(days => $1)
	`, days)
	if err != nil {
		return 0, fmt.Errorf("annotations: prune idempotency keys: %w", err)
	}
	return res.RowsAffected()
}
```
(`make_interval(days => $1)` is type-safe — `$1` is a plain int, no string-concat cast.)

- [ ] **Step 4: Run it; expect PASS**

Run: `cd backend && go test -tags=integration -timeout 240s -run TestAnnotationsRepo_PruneIdempotencyKeys ./internal/query/`
Expected: PASS.

- [ ] **Step 5: Build + full query integration (no regression)**

Run: `cd backend && go build ./... && go test -tags=integration -timeout 300s ./internal/query/`
Expected: all PASS (existing annotations/repo/handler integration + the new prune test).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/query/annotations_repo.go backend/internal/query/annotations_repo_test.go
git commit -m "feat(ann-1): AnnotationsRepo.PruneIdempotencyKeys (D9 retention prune)"
```

---

## Task 2: config env + background pruner + wire into cmd/query

AC#3, AC#4 (config test).

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/query/annotations_pruner.go`
- Modify: `backend/cmd/query/main.go`
- Modify/Create: `backend/internal/config/config_test.go` (config defaults unit test)
- Create: `backend/internal/query/annotations_pruner_test.go` (integration)

- [ ] **Step 1: Add config fields + env parsing**

In `backend/internal/config/config.go`, add to the `Config` struct:

```go
	AnnotationsRetentionDays int           // default 30 — null idempotency_key older than this
	AnnotationsPruneInterval time.Duration // default 24h — how often cmd/query prunes
```

In `FromEnv()`, after the existing TOPO_* block, add (mirror the existing parse-with-default pattern; `strconv`, `time`, `fmt`, `os` are already imported):

```go
	cfg.AnnotationsRetentionDays = 30
	if v := os.Getenv("ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("config: ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS: %w", err)
		}
		cfg.AnnotationsRetentionDays = n
	}

	cfg.AnnotationsPruneInterval = 24 * time.Hour
	if v := os.Getenv("ANNOTATIONS_PRUNE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("config: ANNOTATIONS_PRUNE_INTERVAL: %w", err)
		}
		cfg.AnnotationsPruneInterval = d
	}
```
(Place these assignments where `cfg` already exists and before the final `return cfg, nil`. Match the exact local variable name the function uses for the Config value — likely `cfg`.)

- [ ] **Step 2: Config defaults unit test**

Add to `backend/internal/config/config_test.go` (create if absent — `package config_test` or `package config`; check the existing test package style). It must set `DATABASE_URL` (FromEnv requires it) and clear the two new vars:

```go
func TestFromEnv_AnnotationDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS", "")
	t.Setenv("ANNOTATIONS_PRUNE_INTERVAL", "")
	cfg, err := config.FromEnv()
	require.NoError(t, err)
	assert.Equal(t, 30, cfg.AnnotationsRetentionDays)
	assert.Equal(t, 24*time.Hour, cfg.AnnotationsPruneInterval)
}

func TestFromEnv_AnnotationOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS", "7")
	t.Setenv("ANNOTATIONS_PRUNE_INTERVAL", "1h")
	cfg, err := config.FromEnv()
	require.NoError(t, err)
	assert.Equal(t, 7, cfg.AnnotationsRetentionDays)
	assert.Equal(t, time.Hour, cfg.AnnotationsPruneInterval)
}
```
If `FromEnv` requires other env vars to succeed (e.g. it errors when CLICKHOUSE_DSN is missing), read `FromEnv` and set the minimum env needed so it returns nil error (DATABASE_URL is the known hard requirement; CH DSN is validated in main.go, not FromEnv — confirm). Adapt as needed; keep the two assertions.

- [ ] **Step 3: Run config test; expect FAIL → implement already done in Step 1 → PASS**

Run: `cd backend && go test ./internal/config/`
Expected: PASS (new tests + existing).

- [ ] **Step 4: Create the pruner**

Create `backend/internal/query/annotations_pruner.go`:

```go
package query

import (
	"context"
	"log/slog"
	"time"
)

// AnnotationsPruner periodically nulls expired annotation idempotency keys so the
// partial unique index uq_annotations_idem stays bounded (PLATFORM-ANN-1 / D9).
type AnnotationsPruner struct {
	repo     *AnnotationsRepo
	days     int
	interval time.Duration
}

func NewAnnotationsPruner(repo *AnnotationsRepo, days int, interval time.Duration) *AnnotationsPruner {
	return &AnnotationsPruner{repo: repo, days: days, interval: interval}
}

// Run prunes once immediately, then every interval until ctx is cancelled. A prune
// error is logged and retried on the next tick — it never aborts the loop.
func (p *AnnotationsPruner) Run(ctx context.Context) {
	p.pruneOnce(ctx)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.pruneOnce(ctx)
		}
	}
}

func (p *AnnotationsPruner) pruneOnce(ctx context.Context) {
	n, err := p.repo.PruneIdempotencyKeys(ctx, p.days)
	if err != nil {
		slog.Warn("annotations pruner: prune failed (will retry next tick)", "err", err)
		return
	}
	if n > 0 {
		slog.Info("annotations pruner: nulled expired idempotency keys",
			"rows", n, "retention_days", p.days)
	}
}
```

- [ ] **Step 5: Pruner integration test**

Create `backend/internal/query/annotations_pruner_test.go` (build tag `integration`, `package query_test`). Test that `Run` performs the initial prune then stops on cancel:

```go
//go:build integration

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

func TestAnnotationsPruner_RunPrunesThenStops(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := uuid.MustParse(t1)

	_, err := db.ExecContext(ctx, `
		INSERT INTO annotations (tenant_id, target_type, target_id, kind, payload, ts, idempotency_key, created_at)
		VALUES ($1,'service','checkout','ai_rca','{}'::jsonb, now(), 'old', now() - interval '40 days')
	`, tid)
	require.NoError(t, err)

	// Long interval so only the initial immediate prune runs before we cancel.
	pruner := query.NewAnnotationsPruner(repo, 30, time.Hour)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { pruner.Run(runCtx); close(done) }()

	// Poll for the initial prune to land (it runs immediately on Run start).
	require.Eventually(t, func() bool {
		var k *string
		_ = db.QueryRowContext(ctx, `SELECT idempotency_key FROM annotations WHERE target_id='checkout'`).Scan(&k)
		return k == nil
	}, 3*time.Second, 50*time.Millisecond, "initial prune should null the old key")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
	assert.True(t, true) // reaching here = Run stopped cleanly
}
```

- [ ] **Step 6: Wire the pruner into `cmd/query/main.go`**

In `backend/cmd/query/main.go`, after `db` is opened/pinged and before (or alongside) the server goroutine, start the pruner with a cancellable context; cancel it on shutdown. Concretely:

```go
	// after: ch connected, resolver/router built, before srv.ListenAndServe goroutine
	pruneCtx, prunecancel := context.WithCancel(context.Background())
	pruner := query.NewAnnotationsPruner(query.NewAnnotationsRepo(db),
		cfg.AnnotationsRetentionDays, cfg.AnnotationsPruneInterval)
	go pruner.Run(pruneCtx)
```
Then in the shutdown section (after `<-quit`, around `srv.Shutdown`), add `prunecancel()`:
```go
	<-quit
	prunecancel() // stop the annotations pruner
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil { ... }
```
(`context` is already imported in main.go. `query` + `cfg` are in scope.)

- [ ] **Step 7: Verify**

Run:
```bash
cd backend && go build ./... && go vet ./internal/query/ ./internal/config/ ./cmd/query/
go test ./internal/config/
go test -tags=integration -timeout 300s -run 'TestAnnotationsPruner|TestAnnotationsRepo' ./internal/query/
```
Expected: build/vet clean; config tests pass; pruner + repo integration pass.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go backend/internal/query/annotations_pruner.go backend/internal/query/annotations_pruner_test.go backend/cmd/query/main.go
git commit -m "feat(ann-1): cmd/query background idempotency-key pruner + config (D9)"
```

---

## Task 3: D8 e2e coverage + verification gate + close drift

AC#1, AC#5, AC#4 (gate).

**Files:**
- Modify: `frontend/e2e/annotations.spec.ts`
- Modify: `docs/claude-progress.json` (close D8 + D9)

- [ ] **Step 1: Add the two e2e assertions**

Append two tests to `frontend/e2e/annotations.spec.ts` (keep the existing two + the `loginAs` helper):

```ts
test('AI annotation badge appears on trace detail', async ({ page, request }) => {
  // grab a real trace_id from the seeded data
  const listRes = await request.get('/api/v1/traces?window=1h', {
    headers: { Authorization: 'Bearer test-key-acme' },
  })
  expect(listRes.ok()).toBeTruthy()
  const traces = (await listRes.json()).items ?? (await listRes.json())
  const traceId = (Array.isArray(traces) ? traces[0]?.trace_id : traces?.items?.[0]?.trace_id)
  expect(traceId, 'a seeded trace must exist (run seed-traces)').toBeTruthy()

  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: { target_type: 'trace', target_id: traceId, kind: 'ai_rca', payload: { summary: 'e2e trace' }, ts: new Date().toISOString() },
  })
  expect([200, 201]).toContain(res.status())

  await loginAs(page, 'test-key-acme')
  await page.goto(`/traces/${traceId}`)
  await expect(page.getByTestId('annotation-badge')).toBeVisible({ timeout: 10_000 })
})

test('AI annotation marker appears on a topology node', async ({ page, request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: { target_type: 'service', target_id: 'checkout', kind: 'ai_rca', payload: { summary: 'e2e topo' }, ts: new Date().toISOString() },
  })
  expect([200, 201]).toContain(res.status())

  await loginAs(page, 'test-key-acme')
  await page.goto('/topology?window=1h')
  await expect(page.getByTestId('graph-node-ann-checkout')).toBeVisible({ timeout: 10_000 })
})
```
NOTE: the `/api/v1/traces` response shape — confirm whether it's `{items:[...]}` or a bare array by reading `frontend/src/api/traces.ts` (the list fetcher) and simplify the `traceId` extraction accordingly (the defensive both-shapes code above is a safety net; replace with the real shape once confirmed). The topology test depends on the `checkout` node existing in the graph — ensure `seed-topology` ran and topo-engine ticked (D6 fixed, so it populates).

- [ ] **Step 2: Rebuild the query image (it changed in Task 2) + ensure seeded data**

Per the `make up` doesn't `--build` lesson, the query binary changed (pruner), so rebuild it. The frontend didn't change (e2e is host-run), but the topology/trace data must be present:
```bash
cd /Users/huangbaixun/code_space/openaiops-platform
docker-compose -f deploy/docker-compose.yml build query
docker-compose -f deploy/docker-compose.yml up -d --no-deps query
make seed
(cd backend && go run ./cmd/seed-traces -target localhost:14317 && go run ./cmd/seed-topology -target localhost:14317)
# topo-engine needs a tick to populate topology_edges_v1 for the topology node test:
sleep 20
```

- [ ] **Step 3: Run the annotations e2e (all four tests)**

Run: `cd frontend && npx playwright test annotations.spec.ts`
Expected: 4 passed (service-detail badge, cross-tenant 403, trace-detail badge, topology marker). If the topology marker is flaky because topo-engine hasn't aggregated yet, wait longer (poll `curl -sk "https://localhost/api/v1/topology?window=1h" -H "Authorization: Bearer test-key-acme"` until `checkout` appears) then re-run. If the trace test can't find a trace_id, confirm `/api/v1/traces` returns seeded data.

- [ ] **Step 4: Full regression gate**

```bash
cd backend && go test ./... && go test -tags=integration -timeout 360s ./internal/query/ ./internal/config/
cd frontend && npx vitest run && npm run build && npx playwright test
```
Expected: backend unit + integration green; frontend vitest + build + full Playwright (existing 34 + 2 new annotations tests = 36) green.

- [ ] **Step 5: Close D8 + D9 in progress.json**

Edit `docs/claude-progress.json`. For the `known_drift` entries `tracked_in: "D8"` and `tracked_in: "D9"`, set each `"severity"` to `"resolved"` and prepend `RESOLVED 2026-05-31 (PLATFORM-ANN-1): ` to each `"item"` (D8: e2e now asserts trace-detail badge + topology node marker; D9: cmd/query background pruner nulls idempotency keys older than 30d, bounding uq_annotations_idem). Keep valid JSON (`python3 -m json.tool docs/claude-progress.json >/dev/null`).

- [ ] **Step 6: Commit**

```bash
git add frontend/e2e/annotations.spec.ts docs/claude-progress.json
git commit -m "test(ann-1): e2e badge on trace-detail + topology node; close D8 + D9"
```

- [ ] **Step 7: Hand off to verification-before-completion → finishing-a-development-branch.**

---

## Self-review notes

- **Spec coverage:** PruneIdempotencyKeys + integration → T1; config + pruner + wiring + config test → T2; D8 e2e + gate + close drift → T3. All 5 ACs traced.
- **Placeholder scan:** no TBD/"handle errors". Each code step has full code. The two "confirm the real shape/fields" notes (AnnotationInput fields, /api/v1/traces response) are explicit read-and-adapt instructions with a working default shown, not placeholders.
- **Type consistency:** `PruneIdempotencyKeys(ctx, days int) (int64, error)` defined T1, used by `AnnotationsPruner.pruneOnce` T2. `NewAnnotationsPruner(repo *AnnotationsRepo, days int, interval time.Duration) *AnnotationsPruner` + `Run(ctx)` consistent T2 ↔ main.go wiring. Config fields `AnnotationsRetentionDays int` / `AnnotationsPruneInterval time.Duration` consistent across config.go, config_test, main.go.
- **Out-of-scope respected:** prune only nulls keys (no delete), one global window (no per-tenant), no schema change, no ADR, no metrics UI.
