package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// ListParams is the parsed/validated input to TracesRepo.List. The handler
// constructs this from the HTTP query string after whitelisting Sort/Order;
// arbitrary user input cannot reach the SQL template.
type ListParams struct {
	TsFrom        time.Time
	TsTo          time.Time
	Service       string
	Operation     string
	MinDurationMs float64
	Limit         int
	Offset        int
	Sort          string // "ts" | "duration"
	Order         string // "asc" | "desc"
}

type TracesRepo struct{ ch *chquery.Conn }

func NewTracesRepo(ch *chquery.Conn) *TracesRepo { return &TracesRepo{ch: ch} }

// allowedSort maps the API-visible sort key to a CH expression. Strict
// whitelist; arbitrary user input cannot reach the SQL template.
var allowedSort = map[string]string{
	"ts":       "start_ts",
	"duration": "approx_duration_ns",
}

var allowedOrder = map[string]string{"asc": "ASC", "desc": "DESC"}

const listSQLTemplate = `
SELECT
    trace_id,
    argMin(service,   ts) AS root_service,
    argMin(operation, ts) AS root_operation,
    min(ts)               AS start_ts,
    sum(duration)         AS approx_duration_ns,
    count()               AS span_count,
    arraySlice(groupUniqArray(service), 1, 10) AS services
FROM traces_v1
WHERE tenant_id = ?
  AND ts >= ? AND ts < ?
  AND (? = '' OR service   = ?)
  AND (? = '' OR operation = ?)
  AND duration >= ?
GROUP BY trace_id
ORDER BY %s %s
LIMIT ? OFFSET ?
`

// List returns up to p.Limit aggregated trace rows for the tenant in ctx.
// hasMore uses a limit+1 trick to avoid an extra count() round-trip.
func (r *TracesRepo) List(ctx context.Context, p ListParams) ([]TraceListItem, bool, error) {
	sortExpr, ok1 := allowedSort[p.Sort]
	orderExpr, ok2 := allowedOrder[p.Order]
	if !ok1 || !ok2 {
		return nil, false, errors.New("invalid sort/order")
	}
	sqlStr := fmt.Sprintf(listSQLTemplate, sortExpr, orderExpr)
	minDur := uint64(p.MinDurationMs * 1_000_000)

	// limit+1 trick to populate has_more without an extra count query.
	rows, err := r.ch.Query(ctx, sqlStr,
		p.TsFrom, p.TsTo,
		p.Service, p.Service,
		p.Operation, p.Operation,
		minDur,
		p.Limit+1, p.Offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var items []TraceListItem
	for rows.Next() {
		var it TraceListItem
		if err := rows.Scan(&it.TraceID, &it.RootService, &it.RootOperation,
			&it.StartTs, &it.DurationNs, &it.SpanCount, &it.Services); err != nil {
			return nil, false, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > p.Limit
	if hasMore {
		items = items[:p.Limit]
	}
	return items, hasMore, nil
}
