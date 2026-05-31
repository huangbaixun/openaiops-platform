package topoengine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

// lastCompletedBucket returns max(ts_bucket) in topology_edges_v1 for the
// tenant in ctx, or zero time if none. Used by Catchup to decide replay start.
// Runs as a tenant-scoped chquery.Conn query (PLATFORM-TOPO-1 / ADR-0005) — the
// `tenant_id = ?` placeholder + custom_tenant_id are injected by MustTenantScope,
// so the tenant_isolation Row Policy passes for the real tenant.
func (e *Engine) lastCompletedBucket(ctx context.Context) time.Time {
	row := e.deps.CH.QueryRow(ctx,
		`SELECT max(ts_bucket) FROM topology_edges_v1 FINAL WHERE tenant_id = ?`)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		// io.EOF / empty = "no buckets yet" (legitimate zero). Other errors also
		// collapse to zero for best-effort catchup; real CH outages surface via
		// the tick-failure metric at the tick layer.
		if !errors.Is(err, io.EOF) {
			slog.Warn("topoengine: lastCompletedBucket scan error (treating as zero)", "err", err)
		}
		return time.Time{}
	}
	return t
}
