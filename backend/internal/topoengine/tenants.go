package topoengine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// activeTenants returns every registered tenant id from the PG tenants table
// (the authoritative registry) via the engine's PG connection. PLATFORM-TOPO-1
// / ADR-0005 replaced the chquery.AdminConn discovery (SELECT DISTINCT tenant_id
// FROM traces_v1), which the tenant_isolation Row Policy filtered to zero rows
// (D6). Idle tenants are included and aggregate to zero rows — acceptable at the
// current scale (a future optimization may pre-filter to tenants with traces in
// the window).
func (e *Engine) activeTenants(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := e.deps.PG.QueryContext(ctx, `SELECT id FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("topoengine: list tenants from pg: %w", err)
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var tid uuid.UUID
		if err := rows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("topoengine: scan tenant id: %w", err)
		}
		out = append(out, tid)
	}
	return out, rows.Err()
}
