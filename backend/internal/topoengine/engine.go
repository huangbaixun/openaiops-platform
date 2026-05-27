package topoengine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
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

// RunBucket fans out per-tenant aggregation for the given closed bucket
// using an errgroup limited to cfg.TenantConcurrency. A single tenant
// failure is logged + counted but does not abort other tenants — the
// errgroup callbacks always return nil so g.Wait() never short-circuits
// (and the inherited gctx is never cancelled by a sibling failure).
//
// adminCtx must carry NO tenant — discovery uses chquery.AdminConn. Each
// per-tenant work unit derives its own tctx via auth.WithTenant.
func (e *Engine) RunBucket(adminCtx context.Context, bucket time.Time) error {
	tenants, err := e.activeTenants(adminCtx, bucket)
	if err != nil {
		return fmt.Errorf("topoengine: list tenants: %w", err)
	}

	g, gctx := errgroup.WithContext(adminCtx)
	g.SetLimit(e.cfg.TenantConcurrency)

	for _, tid := range tenants {
		tid := tid // capture per iteration
		g.Go(func() error {
			tctx := auth.WithTenant(gctx, tid, "topo-engine")
			if err := e.runBucketForTenant(tctx, bucket); err != nil {
				e.metrics.TenantFailedTotal.WithLabelValues(tid.String()).Inc()
				slog.Error("topo-engine tenant failed",
					"tenant", tid.String(), "bucket", bucket, "err", err)
				return nil // do not propagate — sibling tenants must continue
			}
			e.metrics.TenantsProcessed.Inc()
			return nil
		})
	}
	_ = g.Wait()
	e.metrics.BucketLagSeconds.Set(time.Since(bucket).Seconds())
	return nil
}

// Catchup replays missing buckets per tenant from
// max(last_completed+1m, now-CatchupMax) up to ClosedBucketAt(now).
// Sequential per-tenant (no errgroup) so CH doesn't see a stampede of
// historical INSERTs on cold start. Per-bucket failures are logged +
// counted but do not stop the rest of the replay for the same tenant or
// for sibling tenants.
//
// adminCtx must carry NO tenant. Tenant discovery uses chquery.AdminConn
// (its sentinel custom_tenant_id requires an operator-managed CH user
// exempted from the tenant_isolation Row Policy to actually see all
// tenants in prod — tracked as a known drift). For test environments where
// the CH user is policy-bound, see CatchupTenant for the per-tenant entry
// point that bypasses discovery.
func (e *Engine) Catchup(adminCtx context.Context) error {
	since := time.Now().UTC().Add(-e.cfg.CatchupMax)
	tenants, err := e.activeTenants(adminCtx, since)
	if err != nil {
		return fmt.Errorf("topoengine: catchup: list tenants: %w", err)
	}
	for _, tid := range tenants {
		// Per-tenant errors are swallowed inside CatchupTenant; the outer Catchup
		// returns nil so the engine keeps ticking even after a bad tenant.
		_ = e.CatchupTenant(adminCtx, tid)
	}
	return nil
}

// CatchupTenant replays the missing-bucket window for ONE tenant. Exposed
// so callers (and integration tests) can skip the AdminConn-based tenant
// discovery path that the prod operator must enable separately.
//
// adminCtx must carry NO tenant; CatchupTenant derives the per-tenant ctx
// via auth.WithTenant. Per-bucket failures are logged + counted but never
// abort the rest of the per-tenant replay — Catchup callers see nil unless
// the initial state read panics.
func (e *Engine) CatchupTenant(adminCtx context.Context, tid uuid.UUID) error {
	tctx := auth.WithTenant(adminCtx, tid, "topo-engine")
	now := time.Now().UTC()
	end := ClosedBucketAt(now)
	last := e.lastCompletedBucket(tctx)

	var start time.Time
	switch {
	case last.IsZero(), end.Sub(last) > e.cfg.CatchupMax:
		// First boot OR gap wider than CatchupMax — replay the full window.
		// Older buckets are accepted as loss per ADR-0002 / SLICE-3 design.
		// NB: lastCompletedBucket may return CH's DateTime default (1970-01-01)
		// rather than the Go zero value for empty result sets — both fall
		// into this branch via the second predicate.
		start = end.Add(-e.cfg.CatchupMax).Truncate(time.Minute)
	default:
		// Resume from the bucket immediately after the last completed one.
		start = last.Add(time.Minute)
	}

	for b := start; !b.After(end); b = b.Add(time.Minute) {
		if err := e.runBucketForTenant(tctx, b); err != nil {
			e.metrics.TenantFailedTotal.WithLabelValues(tid.String()).Inc()
			slog.Error("topo-engine catchup bucket failed",
				"tenant", tid.String(), "bucket", b, "err", err)
		}
	}
	return nil
}
