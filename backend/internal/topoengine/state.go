package topoengine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// lastCompletedBucket returns max(ts_bucket) in topology_edges_v1 for the
// tenant in ctx, or zero time if none. Used by Catchup to decide replay start.
// Reads via chquery.AdminConn (AdminMaxBucket whitelist) with tenant_id bound
// as a SQL arg — no need for chquery.Conn here.
func (e *Engine) lastCompletedBucket(ctx context.Context) time.Time {
	tid, err := auth.TenantID(ctx)
	if err != nil {
		panic("topoengine: lastCompletedBucket called without tenant in ctx: " + err.Error())
	}
	row := e.deps.Admin.AdminQueryRow(ctx, chquery.AdminMaxBucket, tid.String())
	var t time.Time
	if err := row.Scan(&t); err != nil {
		// io.EOF / empty result = "no buckets yet, first boot" (legitimate zero).
		// Other errors (driver / network) ALSO collapse to zero here for best-effort
		// catchup behavior; the caller treats zero as "replay from now - CatchupMax".
		// Real CH outages will surface via topo_engine_tick_failed_total at the tick layer.
		if !errors.Is(err, io.EOF) {
			slog.Warn("topoengine: lastCompletedBucket scan error (treating as zero)",
				"err", err)
		}
		return time.Time{}
	}
	return t
}
