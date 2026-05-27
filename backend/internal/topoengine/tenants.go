package topoengine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// activeTenants returns distinct tenant_id values seen in traces_v1 since
// the given time. Uses chquery.AdminConn (no tenant ctx required).
//
// Returns uuid.UUID values directly — the project's tenant package exposes
// no ID alias, so callers handle bare UUIDs (matches auth.TenantID's signature).
// Non-UUID values in the column are skipped defensively rather than failing
// the whole tick.
func (e *Engine) activeTenants(ctx context.Context, since time.Time) ([]uuid.UUID, error) {
	rows, err := e.deps.Admin.AdminQuery(ctx, chquery.AdminListTenants, since)
	if err != nil {
		return nil, fmt.Errorf("topoengine: list tenants: %w", err)
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("topoengine: scan tenant_id: %w", err)
		}
		tid, err := uuid.Parse(s)
		if err != nil {
			slog.Warn("topoengine: skipping non-UUID tenant_id in traces_v1",
				"tenant_id", s, "err", err)
			continue
		}
		out = append(out, tid)
	}
	return out, nil
}
