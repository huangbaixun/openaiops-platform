package logingest

import (
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// logsToRows flattens a plog.Logs batch into []LogRow.
// SDK-declared "tenant.id" and "service.name" resource attributes are stripped:
// tenant_id is server-stamped by the caller after Bearer→PG resolve;
// service.name is promoted to the Service field.
func logsToRows(ld plog.Logs) []LogRow {
	var out []LogRow
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		resAttrs := mapAttrs(rl.Resource().Attributes())
		delete(resAttrs, "tenant.id")
		service := resAttrs["service.name"]
		delete(resAttrs, "service.name")

		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			recs := sls.At(j).LogRecords()
			for k := 0; k < recs.Len(); k++ {
				rec := recs.At(k)
				// pcommon.Timestamp(0).AsTime() == 1970-01-01 (Unix epoch),
				// NOT Go's zero time. Check the raw uint64 for "unset".
				var ts, observed time.Time
				if rec.Timestamp() != 0 {
					ts = rec.Timestamp().AsTime()
				}
				if rec.ObservedTimestamp() != 0 {
					observed = rec.ObservedTimestamp().AsTime()
				}
				if ts.IsZero() {
					ts = observed
				}
				if observed.IsZero() {
					observed = time.Now().UTC()
				}
				tid := rec.TraceID()
				sid := rec.SpanID()
				out = append(out, LogRow{
					Ts:                 ts,
					ObservedTs:         observed,
					Service:            service,
					SeverityText:       rec.SeverityText(),
					SeverityNumber:     uint8(int(rec.SeverityNumber())),
					Body:               bodyAsString(rec.Body()),
					TraceID:            hexIfNonZero(tid[:]),
					SpanID:             hexIfNonZero(sid[:]),
					TraceFlags:         uint8(uint32(rec.Flags()) & 0xFF),
					ResourceAttributes: resAttrs,
					Attributes:         mapAttrs(rec.Attributes()),
				})
			}
		}
	}
	return out
}

// hexIfNonZero returns the hex-encoded string of b only when at least one byte
// is non-zero; otherwise returns "". This prevents emitting all-zero trace_id /
// span_id strings when the SDK did not set them.
func hexIfNonZero(b []byte) string {
	for _, x := range b {
		if x != 0 {
			return hex.EncodeToString(b)
		}
	}
	return ""
}

// mapAttrs converts a pcommon.Map to map[string]string using AsString() for
// every value. Intentionally duplicated from internal/ingest/spans.go — the
// two packages do not import each other; consolidation deferred to a future cleanup.
func mapAttrs(attrs pcommon.Map) map[string]string {
	out := make(map[string]string, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

// bodyAsString renders a pcommon.Value to a string suitable for CH storage.
// String, int, bool, double are rendered natively; Map and Slice are
// JSON-marshalled via AsRaw() — json.Marshal on map[string]any is deterministic
// (alphabetical key order), so tests can assert exact JSON strings.
func bodyAsString(v pcommon.Value) string {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.AsString()
	case pcommon.ValueTypeInt:
		return strconv.FormatInt(v.Int(), 10)
	case pcommon.ValueTypeBool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case pcommon.ValueTypeDouble:
		return strconv.FormatFloat(v.Double(), 'f', -1, 64)
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		b, err := json.Marshal(v.AsRaw())
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return v.AsString()
	}
}
