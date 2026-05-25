package ingest

import (
	"encoding/hex"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// spansToRows flattens a ptrace.Traces batch into []SpanRow.
// SDK-declared "tenant.id" resource attribute is intentionally dropped;
// tenant_id is server-stamped by the caller after Bearer→PG resolve.
func spansToRows(td ptrace.Traces) []SpanRow {
	var out []SpanRow
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resAttrs := mapAttrs(rs.Resource().Attributes())
		delete(resAttrs, "tenant.id")
		service := stringOrEmpty(rs.Resource().Attributes(), "service.name")
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				sp := spans.At(k)
				attrs := mapAttrs(sp.Attributes())
				if name := ss.Scope().Name(); name != "" {
					attrs["scope.name"] = name
				}
				if ver := ss.Scope().Version(); ver != "" {
					attrs["scope.version"] = ver
				}
				traceID := sp.TraceID()
				spanID := sp.SpanID()
				parentID := sp.ParentSpanID()
				out = append(out, SpanRow{
					TraceID:            hex.EncodeToString(traceID[:]),
					SpanID:             hex.EncodeToString(spanID[:]),
					ParentSpanID:       hex.EncodeToString(parentID[:]),
					Service:            service,
					Operation:          sp.Name(),
					Ts:                 sp.StartTimestamp().AsTime(),
					DurationNs:         uint64(sp.EndTimestamp() - sp.StartTimestamp()),
					Status:             sp.Status().Code().String(),
					SpanKind:           sp.Kind().String(),
					ResourceAttributes: resAttrs,
					Attributes:         attrs,
				})
			}
		}
	}
	return out
}

func mapAttrs(am pcommon.Map) map[string]string {
	out := make(map[string]string, am.Len())
	am.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

func stringOrEmpty(m pcommon.Map, key string) string {
	v, ok := m.Get(key)
	if !ok {
		return ""
	}
	return v.AsString()
}
