package topoengine

import (
	"context"
	"fmt"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// edgesSQL is Pass A — derive service-to-service edges from traces_v1 spans
// in [bucketStart, bucketEnd).
//
// Two edge kinds emitted in one statement:
//
//   - Internal: parent and child are in different services. Edge is parent
//     service (caller) -> child service (callee), callee_kind=service.
//
//   - External: a Client span whose parent is missing (lost) or in the SAME
//     service, AND that carries a peer.service / db.system / messaging.system
//     / http.host attribute. Edge is the Client span's own service (caller) ->
//     the peer string (callee), callee_kind=external.
//
// SETTINGS join_use_nulls = 1 is REQUIRED. Without it, CH's LEFT JOIN
// returns the column DEFAULT VALUE for unmatched right-side rows. For
// LowCardinality(String) that default is '' (not NULL), so `a.service IS NULL`
// would never fire and the "lost parent" external edge branch would silently
// vanish. The integration test's `checkout -> redis` assertion is the canary
// for this regression.
//
// The multiIf branches on the SAME predicate as the WHERE clause's first
// disjunct so caller_service / callee_service / callee_kind stay in lockstep
// with which edge type each output row represents.
//
// MustTenantScope requires (tenant_id, as first INSERT column AND the SELECT
// must reference `tenant_id = ?` somewhere — both satisfied by the literal
// `?` on the tenant_id column projection and the `b.tenant_id = ?` filter.
const edgesSQL = `
INSERT INTO topology_edges_v1 (
    tenant_id, ts_bucket,
    caller_service, caller_kind,
    callee_service, callee_kind,
    calls, errors, p95_duration
)
SELECT
    ? AS tenant_id,
    toStartOfMinute(b.ts) AS ts_bucket,
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, a.service,
        b.service
    ) AS caller_service,
    'service' AS caller_kind,
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, b.service,
        coalesce(
            nullIf(b.attributes['peer.service'], ''),
            nullIf(b.attributes['db.system'], ''),
            nullIf(b.attributes['messaging.system'], ''),
            nullIf(b.attributes['http.host'], ''),
            'unknown-external'
        )
    ) AS callee_service,
    multiIf(
        a.service IS NOT NULL AND a.service != b.service, 'service',
        'external'
    ) AS callee_kind,
    count() AS calls,
    countIf(b.status = 'Error') AS errors,
    toUInt64(quantile(0.95)(b.duration)) AS p95_duration
FROM traces_v1 b
LEFT JOIN traces_v1 a
    ON  a.tenant_id = b.tenant_id
    AND a.trace_id  = b.trace_id
    AND a.span_id   = b.parent_span_id
WHERE b.tenant_id = ?
  AND b.ts >= ? AND b.ts < ?
  AND (
    (a.service IS NOT NULL AND a.service != b.service)
    OR
    (b.span_kind = 'Client' AND b.parent_span_id != ''
     AND (a.service IS NULL OR a.service = b.service)
     AND (
       has(b.attributes, 'peer.service') OR
       has(b.attributes, 'db.system') OR
       has(b.attributes, 'messaging.system') OR
       has(b.attributes, 'http.host')
     ))
  )
GROUP BY caller_service, callee_service, callee_kind, ts_bucket
SETTINGS join_use_nulls = 1
`

// runPassEdges executes Pass A for the tenant in ctx, aggregating the
// closed bucket [bucketStart, bucketStart+1min).
//
// The tenant_id is bound twice — once as the projected column value (so the
// INSERT shape passes MustTenantScope's `(tenant_id,` first-column check)
// and once in the WHERE filter (so the SELECT shape passes the `tenant_id = ?`
// check). The MustTenantScope wrapper itself ALSO prepends tenant_id as the
// very first arg, so callers pass two ? in the SQL but only the two
// bucketStart/bucketEnd args here — MustTenantScope adds the leading tenant_id.
//
// Wait — re-read scope.go: MustTenantScope ALWAYS prepends tenant_id once.
// Our SQL has TWO `?` for tenant_id (projection + filter) plus TWO for the
// bucket range = 4 placeholders. We pass 3 args (tid, bucketStart, bucketEnd)
// and MustTenantScope prepends 1 more = 4 total. Order:
//
//	args[0] = tid (prepended by MustTenantScope) -> projection `?`
//	args[1] = tid (we pass)                       -> filter `b.tenant_id = ?`
//	args[2] = bucketStart                          -> `b.ts >= ?`
//	args[3] = bucketEnd                            -> `b.ts < ?`
func (e *Engine) runPassEdges(ctx context.Context, bucketStart time.Time) error {
	tid, err := auth.TenantID(ctx)
	if err != nil {
		return fmt.Errorf("topoengine: edges: tenant missing in ctx: %w", err)
	}
	bucketEnd := bucketStart.Add(time.Minute)
	start := time.Now()
	if err := e.deps.CH.Exec(ctx, edgesSQL,
		tid.String(),
		bucketStart, bucketEnd,
	); err != nil {
		return fmt.Errorf("topoengine: edges exec: %w", err)
	}
	e.metrics.PassDuration.WithLabelValues("edges").Observe(time.Since(start).Seconds())
	e.metrics.EdgesWritten.WithLabelValues(tid.String()).Inc()
	return nil
}
