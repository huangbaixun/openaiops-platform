package ingest

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpansToRows_DropsTenantIDResourceAttr(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "frontend")
	rs.Resource().Attributes().PutStr("tenant.id", "evil-spoof")
	ss := rs.ScopeSpans().AppendEmpty()
	sp := ss.Spans().AppendEmpty()
	sp.SetName("HTTP GET /")
	sp.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3}))
	sp.SetSpanID(pcommon.SpanID([8]byte{1}))
	sp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 1)))
	sp.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 1001)))

	rows := spansToRows(td)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if _, has := rows[0].ResourceAttributes["tenant.id"]; has {
		t.Fatal("tenant.id resource attr must be dropped")
	}
	if rows[0].Service != "frontend" {
		t.Fatalf("service = %q", rows[0].Service)
	}
	if rows[0].DurationNs != 1000 {
		t.Fatalf("duration = %d", rows[0].DurationNs)
	}
}

func TestSpansToRows_EmptyReturnsZero(t *testing.T) {
	if rows := spansToRows(ptrace.NewTraces()); len(rows) != 0 {
		t.Fatalf("want 0, got %d", len(rows))
	}
}

func TestSpansToRows_InjectsScopeNameVersion(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "frontend")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("mylib")
	ss.Scope().SetVersion("v1.2.3")
	sp := ss.Spans().AppendEmpty()
	sp.SetName("op")
	sp.SetTraceID(pcommon.TraceID([16]byte{1}))
	sp.SetSpanID(pcommon.SpanID([8]byte{1}))
	sp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 0)))
	sp.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 1)))

	rows := spansToRows(td)
	if len(rows) != 1 {
		t.Fatalf("want 1, got %d", len(rows))
	}
	if rows[0].Attributes["scope.name"] != "mylib" {
		t.Fatalf("scope.name = %q", rows[0].Attributes["scope.name"])
	}
	if rows[0].Attributes["scope.version"] != "v1.2.3" {
		t.Fatalf("scope.version = %q", rows[0].Attributes["scope.version"])
	}
}
