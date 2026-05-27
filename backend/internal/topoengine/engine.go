package topoengine

import (
	"context"
	"time"
)

// RunOnce processes the given closed bucket for the tenant in ctx.
// At this stage only Pass A (edges) runs; Pass B (service_stats) is added in T5.
//
// ctx MUST carry a tenant via auth.WithTenant; the per-tenant chquery.Conn
// enforces tenant scoping on the underlying CH access. The bucket argument is
// the START of the 1-minute window to aggregate — callers should compute it
// via ClosedBucketAt(now) to ensure no in-flight ingest writes can land in it.
func (e *Engine) RunOnce(ctx context.Context, bucket time.Time) error {
	return e.runPassEdges(ctx, bucket)
}
