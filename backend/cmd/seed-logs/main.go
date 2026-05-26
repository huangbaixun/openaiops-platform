package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	target := flag.String("target", envDefault("LOG_INGESTER_OTLP_GRPC_HOST_PORT", "127.0.0.1:4327"), "log ingester gRPC addr")
	apiKey := flag.String("key", envDefault("OTEL_LOG_BEARER", "test-key-acme"), "bearer api key")
	count := flag.Int("count", 5, "number of log records to emit")
	traceID := flag.String("trace_id", "4bf92f3577b34da6a3ce929d0e0e4736", "shared trace_id for cross-jump demo")
	spanID := flag.String("span_id", "00f067aa0ba902b7", "shared span_id for span-scoping demo")
	flag.Parse()

	conn, err := grpc.NewClient(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	cli := plogotlp.NewGRPCClient(conn)

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "cartservice")
	rl.Resource().Attributes().PutStr("service.namespace", "demo")

	tid := decodeHex(*traceID)
	sid := decodeHex(*spanID)

	severities := []struct {
		text   string
		number plog.SeverityNumber
	}{
		{"INFO", plog.SeverityNumberInfo},
		{"INFO", plog.SeverityNumberInfo},
		{"WARN", plog.SeverityNumberWarn},
		{"ERROR", plog.SeverityNumberError},
		{"FATAL", plog.SeverityNumberFatal},
	}

	for i := 0; i < *count; i++ {
		rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Duration(-i) * time.Second)))
		sev := severities[i%len(severities)]
		rec.SetSeverityText(sev.text)
		rec.SetSeverityNumber(sev.number)
		body := fmt.Sprintf(`{"event":"checkout","status":"%s","attempt":%d}`, sev.text, i+1)
		rec.Body().SetStr(body)
		var ta pcommon.TraceID
		copy(ta[:], tid)
		rec.SetTraceID(ta)
		var sa pcommon.SpanID
		copy(sa[:], sid)
		rec.SetSpanID(sa)
		rec.Attributes().PutInt("http.status_code", 500)
	}

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+*apiKey)
	if _, err := cli.Export(ctx, plogotlp.NewExportRequestFromLogs(ld)); err != nil {
		log.Fatalf("export: %v", err)
	}
	fmt.Printf("seeded %d log records to %s for key %s (trace_id=%s span_id=%s)\n",
		*count, *target, *apiKey, *traceID, *spanID)
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		if !contains(v, ':') {
			return "127.0.0.1:" + v
		}
		return v
	}
	return d
}

func contains(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func decodeHex(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		_, err := fmt.Sscanf(s[2*i:2*i+2], "%02x", &b)
		if err != nil {
			panic(err)
		}
		out[i] = b
	}
	return out
}
