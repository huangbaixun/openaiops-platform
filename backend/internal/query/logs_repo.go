package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// LogsListParams is the parsed/validated input to LogsRepo.List.
// TenantID is NOT included — chquery.Conn prepends it from context via MustTenantScope.
type LogsListParams struct {
	Service      []string
	Severity     []string
	TsFrom       time.Time
	TsTo         time.Time
	TraceID      string
	SpanID       string
	BodyContains string
	Limit        int
	Offset       int
}

// LogItem is one row from logs_v1.
type LogItem struct {
	Ts                 time.Time         `json:"ts"`
	ObservedTs         time.Time         `json:"observed_ts"`
	Service            string            `json:"service"`
	SeverityText       string            `json:"severity_text"`
	SeverityNumber     uint8             `json:"severity_number"`
	Body               string            `json:"body"`
	TraceID            string            `json:"trace_id"`
	SpanID             string            `json:"span_id"`
	TraceFlags         uint8             `json:"trace_flags"`
	ResourceAttributes map[string]string `json:"resource_attributes"`
	Attributes         map[string]string `json:"attributes"`
}

// LogsListResponse is the JSON envelope for the logs list endpoint.
type LogsListResponse struct {
	Items   []LogItem `json:"items"`
	HasMore bool      `json:"has_more"`
}

// LogsRepo queries logs_v1 through chquery.Conn (which enforces tenant scoping).
type LogsRepo struct{ ch *chquery.Conn }

// NewLogsRepo creates a LogsRepo backed by ch.
func NewLogsRepo(ch *chquery.Conn) *LogsRepo { return &LogsRepo{ch: ch} }

// logsListSQL selects rows from logs_v1.
//
// The WHERE clause shape is required by chquery.MustTenantScope:
//   - first predicate must be `tenant_id = ?` (auto-prepended from ctx)
//   - remaining args follow in-order
//
// Array filters use the `length(?) = 0 OR has(?, col)` idiom: pass the slice
// twice so the empty-check short-circuits and has() applies when non-empty.
const logsListSQL = `
SELECT ts, observed_ts, service, severity_text, severity_number, body,
       trace_id, span_id, trace_flags, resource_attributes, attributes
FROM logs_v1
WHERE tenant_id = ?
  AND ts >= ? AND ts < ?
  AND (length(?) = 0 OR has(?, service))
  AND (length(?) = 0 OR has(?, severity_text))
  AND (? = '' OR trace_id = ?)
  AND (? = '' OR span_id  = ?)
  AND (? = '' OR positionUTF8(body, ?) > 0)
ORDER BY ts DESC
LIMIT ? OFFSET ?
`

// List returns up to p.Limit log rows for the tenant in ctx.
// hasMore uses a limit+1 trick to avoid an extra count() round-trip.
func (r *LogsRepo) List(ctx context.Context, p LogsListParams) ([]LogItem, bool, error) {
	if p.Limit <= 0 || p.Limit > 500 {
		return nil, false, errors.New("limit must be in [1,500]")
	}
	if p.Service == nil {
		p.Service = []string{}
	}
	if p.Severity == nil {
		p.Severity = []string{}
	}

	rows, err := r.ch.Query(ctx, logsListSQL,
		p.TsFrom, p.TsTo,
		p.Service, p.Service,
		p.Severity, p.Severity,
		p.TraceID, p.TraceID,
		p.SpanID, p.SpanID,
		p.BodyContains, p.BodyContains,
		p.Limit+1, p.Offset,
	)
	if err != nil {
		return nil, false, fmt.Errorf("logs list: %w", err)
	}
	defer rows.Close()

	out := make([]LogItem, 0, p.Limit)
	for rows.Next() {
		var it LogItem
		if err := rows.Scan(
			&it.Ts, &it.ObservedTs, &it.Service, &it.SeverityText, &it.SeverityNumber, &it.Body,
			&it.TraceID, &it.SpanID, &it.TraceFlags, &it.ResourceAttributes, &it.Attributes,
		); err != nil {
			return nil, false, fmt.Errorf("logs scan: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("logs rows: %w", err)
	}

	hasMore := len(out) > p.Limit
	if hasMore {
		out = out[:p.Limit]
	}
	return out, hasMore, nil
}
