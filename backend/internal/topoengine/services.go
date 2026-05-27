package topoengine

import (
	"context"
	"fmt"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

// servicesSQL is Pass B — per-(service, span_kind) RED aggregation for the
// closed bucket. Used by:
//
//   - Overview cards (Server kind = inbound RED)
//   - /services/:name (both kinds)
//   - topology node sizing (calls/errors drive node weight)
//
// Placeholder accounting (mirrors edges.go pattern):
//
//	args[0] = tid (prepended by MustTenantScope) -> projection `?`
//	args[1] = tid (we pass)                       -> filter `tenant_id = ?`
//	args[2] = bucketStart                          -> `ts >= ?`
//	args[3] = bucketEnd                            -> `ts < ?`
//
// So 4 placeholders in SQL, runPassServices passes 3 explicit args
// (tid, bucketStart, bucketEnd) and MustTenantScope prepends 1 = 4 total.
const servicesSQL = `
INSERT INTO service_stats_v1 (
    tenant_id, ts_bucket, service, span_kind,
    calls, errors, p95_duration
)
SELECT
    ? AS tenant_id,
    toStartOfMinute(ts) AS ts_bucket,
    service,
    span_kind,
    count() AS calls,
    countIf(status = 'Error') AS errors,
    toUInt64(quantile(0.95)(duration)) AS p95_duration
FROM traces_v1
WHERE tenant_id = ?
  AND ts >= ? AND ts < ?
GROUP BY service, span_kind, ts_bucket
`

// runPassServices executes Pass B for the tenant in ctx, aggregating the
// closed bucket [bucketStart, bucketStart+1min).
func (e *Engine) runPassServices(ctx context.Context, bucketStart time.Time) error {
	tid, err := auth.TenantID(ctx)
	if err != nil {
		return fmt.Errorf("topoengine: services: tenant missing in ctx: %w", err)
	}
	bucketEnd := bucketStart.Add(time.Minute)
	start := time.Now()
	if err := e.deps.CH.Exec(ctx, servicesSQL,
		tid.String(),
		bucketStart, bucketEnd,
	); err != nil {
		return fmt.Errorf("topoengine: services exec: %w", err)
	}
	e.metrics.PassDuration.WithLabelValues("services").Observe(time.Since(start).Seconds())
	e.metrics.ServicesWritten.WithLabelValues(tid.String()).Inc()
	return nil
}
