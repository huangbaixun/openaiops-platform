package query

import "time"

type TraceListItem struct {
	TraceID       string    `json:"trace_id"`
	RootService   string    `json:"root_service"`
	RootOperation string    `json:"root_operation"`
	StartTs       time.Time `json:"start_ts"`
	DurationNs    uint64    `json:"duration_ns"`
	SpanCount     uint64    `json:"span_count"`
	Services      []string  `json:"services"`
}

type TraceListResponse struct {
	Items   []TraceListItem `json:"items"`
	HasMore bool            `json:"has_more"`
}

type SpanDetail struct {
	SpanID             string            `json:"span_id"`
	ParentSpanID       string            `json:"parent_span_id"`
	Service            string            `json:"service"`
	Operation          string            `json:"operation"`
	Ts                 time.Time         `json:"ts"`
	DurationNs         uint64            `json:"duration_ns"`
	Status             string            `json:"status"`
	SpanKind           string            `json:"span_kind"`
	ResourceAttributes map[string]string `json:"resource_attributes"`
	Attributes         map[string]string `json:"attributes"`
}

type TraceDetailResponse struct {
	TraceID string       `json:"trace_id"`
	Spans   []SpanDetail `json:"spans"`
}

// ---- SLICE-3: services + topology response shapes ------------------------

// ServicesListItem is one row in GET /v1/services.
type ServicesListItem struct {
	Service          string  `json:"service"`
	InboundCalls     uint64  `json:"inbound_calls"`
	InboundErrors    uint64  `json:"inbound_errors"`
	InboundErrorRate float64 `json:"inbound_error_rate"`
	InboundP95Ms     float64 `json:"inbound_p95_ms"`
	OutboundCalls    uint64  `json:"outbound_calls"`
}

// ServicesListResponse is the JSON envelope for GET /v1/services.
type ServicesListResponse struct {
	Window string             `json:"window"`
	Items  []ServicesListItem `json:"items"`
}

// ServiceDirectionStats describes Server-side (inbound) or Client-side (outbound)
// aggregates for one service.
type ServiceDirectionStats struct {
	Calls     uint64  `json:"calls"`
	Errors    uint64  `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
	P95Ms     float64 `json:"p95_ms"`
}

// Dependency is one peer (either a sibling service or an external system) in a
// service-detail dependency list.
type Dependency struct {
	Peer     string  `json:"peer"`
	PeerKind string  `json:"peer_kind"` // 'service' | 'external'
	Calls    uint64  `json:"calls"`
	Errors   uint64  `json:"errors"`
	P95Ms    float64 `json:"p95_ms"`
}

// ServiceDetailResponse is the JSON envelope for GET /v1/services/{name}.
type ServiceDetailResponse struct {
	Service string `json:"service"`
	Window  string `json:"window"`
	Stats   struct {
		Inbound  ServiceDirectionStats `json:"inbound"`
		Outbound ServiceDirectionStats `json:"outbound"`
	} `json:"stats"`
	Dependencies struct {
		Inbound  []Dependency `json:"inbound"`
		Outbound []Dependency `json:"outbound"`
	} `json:"dependencies"`
}

// TopologyNode is one service node in the topology graph response.
type TopologyNode struct {
	Service string  `json:"service"`
	Kind    string  `json:"kind"` // 'service' | 'external'
	Calls   uint64  `json:"calls"`
	Errors  uint64  `json:"errors"`
	P95Ms   float64 `json:"p95_ms"`
}

// TopologyEdge is one directed caller->callee edge in the topology graph.
type TopologyEdge struct {
	Caller     string  `json:"caller"`
	Callee     string  `json:"callee"`
	CalleeKind string  `json:"callee_kind"`
	Calls      uint64  `json:"calls"`
	Errors     uint64  `json:"errors"`
	P95Ms      float64 `json:"p95_ms"`
}

// TopologyResponse is the JSON envelope for GET /v1/topology.
type TopologyResponse struct {
	Window string         `json:"window"`
	Nodes  []TopologyNode `json:"nodes"`
	Edges  []TopologyEdge `json:"edges"`
}

// WindowToMinutes maps the whitelisted SLICE-3 window enum to total minutes
// for INTERVAL ? MINUTE clauses. Returns -1 for any value not in the whitelist
// (handlers must reject with 400).
func WindowToMinutes(w string) int {
	switch w {
	case "15m":
		return 15
	case "1h":
		return 60
	case "6h":
		return 360
	case "24h":
		return 24 * 60
	}
	return -1
}
