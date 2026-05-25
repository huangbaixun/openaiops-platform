package ingest

import "time"

type SpanRow struct {
	TraceID            string
	SpanID             string
	ParentSpanID       string
	Service            string
	Operation          string
	Ts                 time.Time
	DurationNs         uint64
	Status             string
	SpanKind           string
	ResourceAttributes map[string]string
	Attributes         map[string]string
}
