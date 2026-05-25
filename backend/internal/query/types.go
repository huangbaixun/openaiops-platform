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
