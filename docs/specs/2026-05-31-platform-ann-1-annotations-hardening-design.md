---
date: 2026-05-31
topic: platform-ann-1-annotations-hardening-design
type: feature
status: proposed
features: [PLATFORM-ANN-1]
---

# PLATFORM-ANN-1 design — annotations hardening (close D8 + D9)

## Context

PLATFORM-ASK-2 shipped the AI annotation write-back: a PG `annotations` table, a
`POST/GET /api/v1/annotations` endpoint on `cmd/query`, and an `<AnnotationBadge>`
surfaced on trace-detail, service-detail, and topology nodes. Two low-severity drift
items were tracked at the time:

- **D8** — the e2e (`frontend/e2e/annotations.spec.ts`) only asserts the badge on
  service-detail (+ the cross-tenant 403). The ASK-2 testing strategy promised badge
  coverage on trace-detail and topology nodes too.
- **D9** — the partial unique index `uq_annotations_idem (tenant_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL` is unbounded: idempotency keys are retained forever.
  The "24h dedupe" was satisfied only as a retention floor; nothing prunes old keys.

This feature closes both. They are bundled because both finish ASK-2; neither warrants its
own slice.

## D8 — e2e badge coverage (trace-detail + topology node)

Extend `frontend/e2e/annotations.spec.ts` with two assertions (keeping the existing two):

1. **trace-detail badge.** `GET /api/v1/traces?window=1h` → take the first item's `trace_id`
   → `POST /api/v1/annotations {target_type:'trace', target_id:<trace_id>, kind:'ai_rca', ...}`
   → log in, `goto /traces/<trace_id>` → assert `getByTestId('annotation-badge')` visible.
   (The badge on `TraceDetail` fetches annotations for `(trace, props.traceId)` independent
   of trace-body load, so it renders once the seeded annotation exists.)
2. **topology node marker.** `POST {target_type:'service', target_id:'checkout', ...}` →
   `goto /topology` → assert `getByTestId('graph-node-ann-checkout')` visible. The marker
   (`ServiceGraph.vue:78`) renders for any service in `annByService`; `TopologyPage` loads
   all service annotations into that map. Requires the topology graph to contain a `checkout`
   node — available now that D6 is fixed (topo-engine populates `topology_edges_v1` from the
   seeded data).

Pure test additions; no app code changes for D8. Relative paths + `baseURL` (mirrors the
existing specs). Requires the live stack seeded with traces + topology (already part of the
e2e gate).

## D9 — idempotency-key retention prune

### What to prune

NULL out idempotency keys older than the retention window — **do not delete the annotations**:

```sql
UPDATE annotations
   SET idempotency_key = NULL
 WHERE idempotency_key IS NOT NULL
   AND created_at < now() - ($1 || ' days')::interval
```

Nulling the key removes the row from the partial unique index (`WHERE idempotency_key IS NOT
NULL`), bounding it. Semantics are correct: idempotency exists for short-term retry; beyond the
retention window a re-POST with the same key legitimately creates a new annotation (dedupe no
longer applies). The annotation content is preserved.

### Mechanism (Option A)

A background pruner goroutine in `cmd/query` (the binary that owns the annotations PG handle and
is long-running). It runs once on boot and then on a ticker. Configurable via env, read in
`internal/config`:

- `ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS` (default `30`)
- `ANNOTATIONS_PRUNE_INTERVAL` (default `24h`)

The query exposes a repo method:

```go
// PruneIdempotencyKeys nulls idempotency_key for annotations older than `days` days.
// Maintenance op — intentionally tenant-UNSCOPED (retention across all tenants); it only
// clears keys, never reads or exposes annotation content. Returns rows affected.
func (r *AnnotationsRepo) PruneIdempotencyKeys(ctx context.Context, days int) (int64, error)
```

The pruner is a tiny struct (`annotationsPruner`) with `Run(ctx)` that loops on a `time.Ticker`,
calls `PruneIdempotencyKeys`, logs the affected count (slog), and exits on `ctx.Done()`. Started
from `cmd/query/main.go` in a goroutine alongside the HTTP server, cancelled on shutdown. A prune
failure is logged and retried on the next tick (never crashes the binary).

Rejected: Option B (a one-shot CLI run by ops cron) — the project has no cron infrastructure, so
it would not actually run.

## Error handling

- Prune DB error → logged via slog, swallowed; the next tick retries. The HTTP server is
  unaffected.
- Empty result (nothing old enough) → `0 rows`, no-op, no log noise beyond a debug line.

## Testing strategy

- **D9 integration (dockertest PG):** insert two annotations with idempotency keys, one stamped
  `created_at = now() - 40 days`; call `PruneIdempotencyKeys(ctx, 30)`; assert the old row's
  `idempotency_key` is NULL and the recent one is unchanged; assert affected count = 1; then
  assert a fresh insert reusing the old key now succeeds (dedupe lapsed) while reusing the recent
  key still dedupes.
- **D9 unit:** config defaults (`30` days / `24h`) parse from env; absent env → defaults.
- **D8 e2e:** the two new Playwright assertions above (run against the live seeded stack).
- **Regression:** all existing backend unit + integration + the full Playwright suite stay green.

## Acceptance criteria

1. `frontend/e2e/annotations.spec.ts` asserts the AI badge on trace-detail (seeded trace
   annotation) and the `graph-node-ann-checkout` marker on /topology, in addition to the
   existing service-detail + cross-tenant 403 cases.
2. `AnnotationsRepo.PruneIdempotencyKeys(ctx, days)` nulls `idempotency_key` for annotations
   older than `days`, returns the affected count, preserves annotation content, and is covered by
   a dockertest integration test (old key nulled, recent kept, dedupe lapses for the old key).
3. A background pruner in `cmd/query` runs the prune on boot + every `ANNOTATIONS_PRUNE_INTERVAL`
   (default 24h) with retention `ANNOTATIONS_IDEMPOTENCY_RETENTION_DAYS` (default 30), logging the
   affected count; failures are logged and retried, never crashing the binary; it stops on shutdown.
4. Config defaults are unit-tested; all existing tests + full Playwright suite stay green.
5. known_drift D8 and D9 are closed.

## Out of scope

- Per-tenant retention policies (one global window).
- Deleting annotations (only the key is nulled).
- Annotation table schema changes; any ADR (no architectural decision).
- A prune-stats UI / metrics dashboard.

## Dependencies

None. Builds on PLATFORM-ASK-2 (annotations) and benefits from PLATFORM-TOPO-1 (D6 fix makes
the topology node assertion viable).

## Related files

- `frontend/e2e/annotations.spec.ts` (extend)
- `backend/internal/query/annotations_repo.go` (add `PruneIdempotencyKeys`)
- `backend/internal/query/annotations_pruner.go` (new — pruner goroutine)
- `backend/internal/config/config.go` (two env vars + defaults)
- `backend/cmd/query/main.go` (start the pruner goroutine)
- `backend/internal/query/annotations_repo_test.go` / a new pruner/repo test (integration)
