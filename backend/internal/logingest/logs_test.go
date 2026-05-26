package logingest

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestLogsToRows_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	traceID := pcommon.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	spanID := pcommon.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "cartservice")
	rl.Resource().Attributes().PutStr("service.namespace", "demo")
	rl.Resource().Attributes().PutStr("tenant.id", "evil-spoofed-tenant") // MUST be dropped
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.SetTimestamp(pcommon.NewTimestampFromTime(now))
	rec.SetObservedTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))
	rec.SetSeverityText("ERROR")
	rec.SetSeverityNumber(plog.SeverityNumberError) // 17
	rec.Body().SetStr("checkout failed: insufficient funds")
	rec.SetTraceID(traceID)
	rec.SetSpanID(spanID)
	// DefaultLogRecordFlags.WithIsSampled(true) is available in pdata v1.59.0
	rec.SetFlags(plog.DefaultLogRecordFlags.WithIsSampled(true))
	rec.Attributes().PutStr("http.status_code", "500")

	got := logsToRows(ld)
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	r := got[0]

	want := LogRow{
		Ts:             now,
		ObservedTs:     now.Add(time.Millisecond),
		Service:        "cartservice",
		SeverityText:   "ERROR",
		SeverityNumber: 17,
		Body:           "checkout failed: insufficient funds",
		TraceID:        hex.EncodeToString(traceID[:]),
		SpanID:         hex.EncodeToString(spanID[:]),
		TraceFlags:     1,
		ResourceAttributes: map[string]string{"service.namespace": "demo"},
		Attributes:         map[string]string{"http.status_code": "500"},
	}
	if diff := cmp.Diff(want, r); diff != "" {
		t.Fatalf("row mismatch (-want +got):\n%s", diff)
	}
}

func TestLogsToRows_EmptyTraceIDSpanID(t *testing.T) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	rec.Body().SetStr("noise")
	got := logsToRows(ld)
	if got[0].TraceID != "" || got[0].SpanID != "" {
		t.Fatalf("want empty trace_id/span_id when OTLP IDs are zero, got %q/%q",
			got[0].TraceID, got[0].SpanID)
	}
}

func TestLogsToRows_TimestampFallback(t *testing.T) {
	observed := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.SetObservedTimestamp(pcommon.NewTimestampFromTime(observed))
	rec.Body().SetStr("late arrival")
	got := logsToRows(ld)
	if !got[0].Ts.Equal(observed) {
		t.Fatalf("ts should fall back to observed_ts, got %v", got[0].Ts)
	}
}

func TestBodyAsString_PrimitivesAndMap(t *testing.T) {
	cases := []struct {
		name string
		set  func(v pcommon.Value)
		want string
	}{
		{"string", func(v pcommon.Value) { v.SetStr("hello") }, "hello"},
		{"int", func(v pcommon.Value) { v.SetInt(42) }, "42"},
		{"bool", func(v pcommon.Value) { v.SetBool(true) }, "true"},
		{"double", func(v pcommon.Value) { v.SetDouble(3.14) }, "3.14"},
		{"map", func(v pcommon.Value) {
			m := v.SetEmptyMap()
			m.PutStr("k", "v")
			m.PutInt("n", 1)
		}, `{"k":"v","n":1}`},
		{"slice", func(v pcommon.Value) {
			s := v.SetEmptySlice()
			s.AppendEmpty().SetStr("a")
			s.AppendEmpty().SetInt(2)
		}, `["a",2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := pcommon.NewValueEmpty()
			tc.set(v)
			got := bodyAsString(v)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
