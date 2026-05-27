package query

import (
	"context"
	"fmt"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// ServicesRepo queries service_stats_v1 + topology_edges_v1 through chquery.Conn
// (which enforces tenant scoping via MustTenantScope + Row Policy).
type ServicesRepo struct{ ch *chquery.Conn }

// NewServicesRepo creates a ServicesRepo backed by ch.
func NewServicesRepo(ch *chquery.Conn) *ServicesRepo { return &ServicesRepo{ch: ch} }

// listServicesSQL aggregates per-service inbound (Server) + outbound (Client)
// RED metrics from service_stats_v1 over the requested window.
//
// %s is one of {inbound_calls, inbound_errors, inbound_p95}, whitelisted in List.
// Single `tenant_id = ?` placeholder; MustTenantScope prepends the tid arg.
const listServicesSQL = `
SELECT
    service,
    sumIf(calls,        span_kind = 'Server') AS inbound_calls,
    sumIf(errors,       span_kind = 'Server') AS inbound_errors,
    maxIf(p95_duration, span_kind = 'Server') AS inbound_p95,
    sumIf(calls,        span_kind = 'Client') AS outbound_calls
FROM service_stats_v1 FINAL
WHERE tenant_id = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY service
HAVING inbound_calls > 0 OR outbound_calls > 0
ORDER BY %s DESC
LIMIT ?
`

// List returns the top `limit` services by `sort` over the given window.
//
// sort ∈ {calls, errors, p95}; window resolved by WindowToMinutes (caller
// already validated). Returns an empty slice (not nil) when the tenant has
// no rows in the window.
func (r *ServicesRepo) List(ctx context.Context, window string, limit int, sort string) ([]ServicesListItem, error) {
	mins := WindowToMinutes(window)
	if mins < 0 {
		return nil, fmt.Errorf("services list: invalid window %q", window)
	}
	var orderBy string
	switch sort {
	case "calls":
		orderBy = "inbound_calls"
	case "errors":
		orderBy = "inbound_errors"
	case "p95":
		orderBy = "inbound_p95"
	default:
		return nil, fmt.Errorf("services list: invalid sort %q", sort)
	}
	sqlStr := fmt.Sprintf(listServicesSQL, orderBy)

	// MustTenantScope prepends tid for the `tenant_id = ?` placeholder; we pass
	// mins and limit for the remaining two `?`.
	rows, err := r.ch.Query(ctx, sqlStr, mins, limit)
	if err != nil {
		return nil, fmt.Errorf("services list: %w", err)
	}
	defer rows.Close()

	out := make([]ServicesListItem, 0)
	for rows.Next() {
		var (
			svc                                          string
			inCalls, inErrors, inP95, outCalls           uint64
		)
		if err := rows.Scan(&svc, &inCalls, &inErrors, &inP95, &outCalls); err != nil {
			return nil, fmt.Errorf("services list scan: %w", err)
		}
		var rate float64
		if inCalls > 0 {
			rate = float64(inErrors) / float64(inCalls)
		}
		out = append(out, ServicesListItem{
			Service:          svc,
			InboundCalls:     inCalls,
			InboundErrors:    inErrors,
			InboundErrorRate: rate,
			InboundP95Ms:     nsToMs(inP95),
			OutboundCalls:    outCalls,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("services list rows: %w", err)
	}
	return out, nil
}

// detailSelfSQL fetches per-direction (Server/Client) RED for one service.
const detailSelfSQL = `
SELECT span_kind, sum(calls), sum(errors), max(p95_duration)
FROM service_stats_v1 FINAL
WHERE tenant_id = ? AND service = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY span_kind
`

// detailInboundSQL lists who calls THIS service.
const detailInboundSQL = `
SELECT caller_service AS peer, 'service' AS peer_kind,
       sum(calls), sum(errors), max(p95_duration)
FROM topology_edges_v1 FINAL
WHERE tenant_id = ? AND callee_service = ? AND callee_kind = 'service'
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY caller_service
ORDER BY sum(calls) DESC
LIMIT 50
`

// detailOutboundSQL lists who THIS service calls (services + externals).
const detailOutboundSQL = `
SELECT callee_service AS peer, callee_kind AS peer_kind,
       sum(calls), sum(errors), max(p95_duration)
FROM topology_edges_v1 FINAL
WHERE tenant_id = ? AND caller_service = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY callee_service, callee_kind
ORDER BY sum(calls) DESC
LIMIT 50
`

// Detail returns the full ServiceDetailResponse for name over window, or nil
// when the named service has zero rows under this tenant (handler maps nil→404).
func (r *ServicesRepo) Detail(ctx context.Context, name, window string) (*ServiceDetailResponse, error) {
	mins := WindowToMinutes(window)
	if mins < 0 {
		return nil, fmt.Errorf("services detail: invalid window %q", window)
	}
	resp := &ServiceDetailResponse{Service: name, Window: window}
	resp.Dependencies.Inbound = []Dependency{}
	resp.Dependencies.Outbound = []Dependency{}

	// Self stats. MustTenantScope prepends tid; we pass (name, mins).
	selfRows, err := r.ch.Query(ctx, detailSelfSQL, name, mins)
	if err != nil {
		return nil, fmt.Errorf("services detail self: %w", err)
	}
	defer selfRows.Close()
	for selfRows.Next() {
		var kind string
		var calls, errs, p95 uint64
		if err := selfRows.Scan(&kind, &calls, &errs, &p95); err != nil {
			return nil, fmt.Errorf("services detail self scan: %w", err)
		}
		stats := ServiceDirectionStats{
			Calls:     calls,
			Errors:    errs,
			ErrorRate: safeRate(errs, calls),
			P95Ms:     nsToMs(p95),
		}
		switch kind {
		case "Server":
			resp.Stats.Inbound = stats
		case "Client":
			resp.Stats.Outbound = stats
		}
	}
	if err := selfRows.Err(); err != nil {
		return nil, fmt.Errorf("services detail self rows: %w", err)
	}

	// Inbound peers.
	inRows, err := r.ch.Query(ctx, detailInboundSQL, name, mins)
	if err != nil {
		return nil, fmt.Errorf("services detail inbound: %w", err)
	}
	defer inRows.Close()
	for inRows.Next() {
		var d Dependency
		var p95 uint64
		if err := inRows.Scan(&d.Peer, &d.PeerKind, &d.Calls, &d.Errors, &p95); err != nil {
			return nil, fmt.Errorf("services detail inbound scan: %w", err)
		}
		d.P95Ms = nsToMs(p95)
		resp.Dependencies.Inbound = append(resp.Dependencies.Inbound, d)
	}
	if err := inRows.Err(); err != nil {
		return nil, fmt.Errorf("services detail inbound rows: %w", err)
	}

	// Outbound peers.
	outRows, err := r.ch.Query(ctx, detailOutboundSQL, name, mins)
	if err != nil {
		return nil, fmt.Errorf("services detail outbound: %w", err)
	}
	defer outRows.Close()
	for outRows.Next() {
		var d Dependency
		var p95 uint64
		if err := outRows.Scan(&d.Peer, &d.PeerKind, &d.Calls, &d.Errors, &p95); err != nil {
			return nil, fmt.Errorf("services detail outbound scan: %w", err)
		}
		d.P95Ms = nsToMs(p95)
		resp.Dependencies.Outbound = append(resp.Dependencies.Outbound, d)
	}
	if err := outRows.Err(); err != nil {
		return nil, fmt.Errorf("services detail outbound rows: %w", err)
	}

	// 404 if nothing at all — never leak existence of another tenant's service.
	if resp.Stats.Inbound.Calls == 0 && resp.Stats.Outbound.Calls == 0 &&
		len(resp.Dependencies.Inbound) == 0 && len(resp.Dependencies.Outbound) == 0 {
		return nil, nil
	}
	return resp, nil
}

// nsToMs converts a nanosecond duration to milliseconds as float.
func nsToMs(ns uint64) float64 { return float64(ns) / 1_000_000.0 }

// safeRate returns num/denom or 0 when denom==0 (avoid NaN in JSON).
func safeRate(num, denom uint64) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) / float64(denom)
}
