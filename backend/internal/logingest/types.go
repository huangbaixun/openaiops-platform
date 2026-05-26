package logingest

import "time"

// LogRow is one CH logs_v1 row, post mapping from plog.LogRecord.
// Field order matches the logs_v1 INSERT statement; tenant_id is prepended by
// the consumer at batch.Append time (server-stamped after Bearer resolve).
type LogRow struct {
	Ts                 time.Time
	ObservedTs         time.Time
	Service            string
	SeverityText       string
	SeverityNumber     uint8
	Body               string
	TraceID            string
	SpanID             string
	TraceFlags         uint8
	ResourceAttributes map[string]string
	Attributes         map[string]string
}
