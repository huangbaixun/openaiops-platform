package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// TopologyRepo queries topology_edges_v1 + service_stats_v1 to build the
// service graph response. Tenant scoping is enforced via chquery.Conn
// (MustTenantScope + Row Policy).
type TopologyRepo struct{ ch *chquery.Conn }

// NewTopologyRepo creates a TopologyRepo backed by ch.
func NewTopologyRepo(ch *chquery.Conn) *TopologyRepo { return &TopologyRepo{ch: ch} }

// topologyEdgesSQL aggregates all edges in the window. Single `tenant_id = ?`
// placeholder; MustTenantScope prepends the tid arg.
const topologyEdgesSQL = `
SELECT
    caller_service, caller_kind,
    callee_service, callee_kind,
    sum(calls)         AS calls,
    sum(errors)        AS errors,
    max(p95_duration)  AS p95_duration
FROM topology_edges_v1 FINAL
WHERE tenant_id = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
GROUP BY caller_service, caller_kind, callee_service, callee_kind
ORDER BY calls DESC
`

// topologyServerStatsSQL aggregates Server-side RED per service. Used to size
// nodes (calls/errors/p95 in the response). Single tenant_id placeholder.
const topologyServerStatsSQL = `
SELECT service,
       sum(calls)        AS calls,
       sum(errors)       AS errors,
       max(p95_duration) AS p95_duration
FROM service_stats_v1 FINAL
WHERE tenant_id = ?
  AND ts_bucket >= now() - INTERVAL ? MINUTE
  AND span_kind = 'Server'
GROUP BY service
`

// Topology builds the graph response over the window. nodeLimit caps the
// number of `service`-kind nodes by inbound call volume; edges referencing
// dropped service nodes are dropped too. `external` nodes are always kept.
func (r *TopologyRepo) Topology(ctx context.Context, window string, nodeLimit int) (*TopologyResponse, error) {
	mins := WindowToMinutes(window)
	if mins < 0 {
		return nil, fmt.Errorf("topology: invalid window %q", window)
	}
	out := &TopologyResponse{
		Window: window,
		Nodes:  []TopologyNode{},
		Edges:  []TopologyEdge{},
	}

	// --- Phase 1: load all edges + accumulate node set. ---
	edgeRows, err := r.ch.Query(ctx, topologyEdgesSQL, mins)
	if err != nil {
		return nil, fmt.Errorf("topology edges: %w", err)
	}
	defer edgeRows.Close()
	type rawEdge struct {
		caller, callerKind, callee, calleeKind string
		calls, errors, p95                     uint64
	}
	var rawEdges []rawEdge
	// nodeKinds tracks kind ('service' or 'external'); 'service' wins ties.
	nodeKinds := map[string]string{}
	for edgeRows.Next() {
		var e rawEdge
		if err := edgeRows.Scan(&e.caller, &e.callerKind, &e.callee, &e.calleeKind, &e.calls, &e.errors, &e.p95); err != nil {
			return nil, fmt.Errorf("topology edges scan: %w", err)
		}
		rawEdges = append(rawEdges, e)
		// Caller is always a service (Pass A only emits service callers).
		if existing, ok := nodeKinds[e.caller]; !ok || existing == "external" {
			nodeKinds[e.caller] = "service"
		}
		if existing, ok := nodeKinds[e.callee]; !ok {
			nodeKinds[e.callee] = e.calleeKind
		} else if existing == "external" && e.calleeKind == "service" {
			nodeKinds[e.callee] = "service"
		}
	}
	if err := edgeRows.Err(); err != nil {
		return nil, fmt.Errorf("topology edges rows: %w", err)
	}

	// --- Phase 2: load Server-side RED for sizing service nodes. ---
	statRows, err := r.ch.Query(ctx, topologyServerStatsSQL, mins)
	if err != nil {
		return nil, fmt.Errorf("topology stats: %w", err)
	}
	defer statRows.Close()
	type stat struct{ calls, errors, p95 uint64 }
	stats := map[string]stat{}
	for statRows.Next() {
		var svc string
		var s stat
		if err := statRows.Scan(&svc, &s.calls, &s.errors, &s.p95); err != nil {
			return nil, fmt.Errorf("topology stats scan: %w", err)
		}
		stats[svc] = s
		// Promote to node set even if no edges (orphan service with traffic).
		if _, ok := nodeKinds[svc]; !ok {
			nodeKinds[svc] = "service"
		}
	}
	if err := statRows.Err(); err != nil {
		return nil, fmt.Errorf("topology stats rows: %w", err)
	}

	// --- Phase 3: rank service nodes by Server calls; cap to nodeLimit. ---
	type nodeCandidate struct {
		name  string
		kind  string
		calls uint64
		stat  stat
	}
	var serviceCandidates []nodeCandidate
	keptExternal := map[string]struct{}{}
	for name, kind := range nodeKinds {
		if kind == "service" {
			serviceCandidates = append(serviceCandidates, nodeCandidate{
				name:  name,
				kind:  "service",
				calls: stats[name].calls,
				stat:  stats[name],
			})
		} else {
			keptExternal[name] = struct{}{}
		}
	}
	sort.Slice(serviceCandidates, func(i, j int) bool {
		if serviceCandidates[i].calls != serviceCandidates[j].calls {
			return serviceCandidates[i].calls > serviceCandidates[j].calls
		}
		return serviceCandidates[i].name < serviceCandidates[j].name
	})
	if len(serviceCandidates) > nodeLimit {
		serviceCandidates = serviceCandidates[:nodeLimit]
	}
	keptServices := map[string]struct{}{}
	for _, c := range serviceCandidates {
		keptServices[c.name] = struct{}{}
		out.Nodes = append(out.Nodes, TopologyNode{
			Service: c.name,
			Kind:    c.kind,
			Calls:   c.stat.calls,
			Errors:  c.stat.errors,
			P95Ms:   nsToMs(c.stat.p95),
		})
	}

	// --- Phase 4: filter edges to surviving nodes. ---
	// Caller must be a kept service; callee must be a kept service OR external.
	for _, e := range rawEdges {
		if _, ok := keptServices[e.caller]; !ok {
			continue
		}
		switch e.calleeKind {
		case "service":
			if _, ok := keptServices[e.callee]; !ok {
				continue
			}
		case "external":
			// always kept; ensure external node is emitted
			if _, ok := keptExternal[e.callee]; !ok {
				continue
			}
		default:
			continue
		}
		out.Edges = append(out.Edges, TopologyEdge{
			Caller:     e.caller,
			Callee:     e.callee,
			CalleeKind: e.calleeKind,
			Calls:      e.calls,
			Errors:     e.errors,
			P95Ms:      nsToMs(e.p95),
		})
	}

	// --- Phase 5: emit external nodes that survived edge filtering. ---
	emittedExternal := map[string]struct{}{}
	for _, edge := range out.Edges {
		if edge.CalleeKind != "external" {
			continue
		}
		if _, dup := emittedExternal[edge.Callee]; dup {
			continue
		}
		emittedExternal[edge.Callee] = struct{}{}
		out.Nodes = append(out.Nodes, TopologyNode{
			Service: edge.Callee,
			Kind:    "external",
			// External nodes have no service_stats_v1 entry by construction.
		})
	}

	return out, nil
}
