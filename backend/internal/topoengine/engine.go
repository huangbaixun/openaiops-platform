package topoengine

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RunOnce processes the given closed bucket for the tenant in ctx.
// Both Pass A (edges) and Pass B (services) run; failures combine via errors.Join
// so a Pass A failure doesn't mask a Pass B failure (and vice versa).
//
// ctx MUST carry a tenant via auth.WithTenant; the per-tenant chquery.Conn
// enforces tenant scoping on the underlying CH access. The bucket argument is
// the START of the 1-minute window to aggregate — callers should compute it
// via ClosedBucketAt(now) to ensure no in-flight ingest writes can land in it.
func (e *Engine) RunOnce(ctx context.Context, bucket time.Time) error {
	var errs []error
	if err := e.runPassEdges(ctx, bucket); err != nil {
		errs = append(errs, fmt.Errorf("edges: %w", err))
	}
	if err := e.runPassServices(ctx, bucket); err != nil {
		errs = append(errs, fmt.Errorf("services: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// runBucketForTenant is the per-tenant unit of work used by RunBucket() in T6.
func (e *Engine) runBucketForTenant(ctx context.Context, bucket time.Time) error {
	return e.RunOnce(ctx, bucket)
}

// ClosedBucketAtNow returns the latest closed bucket as of now — a convenience
// for callers (e.g. cmd/topo-engine) that want it without importing time.
func ClosedBucketAtNow() time.Time { return ClosedBucketAt(time.Now()) }
