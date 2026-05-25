package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	target := flag.String("target", "localhost:4317", "ingester OTLP/gRPC addr")
	key := flag.String("tenant-key", "test-key-acme", "Bearer API key (plaintext)")
	spans := flag.Int("spans", 5, "spans per trace")
	traces := flag.Int("traces", 1, "number of traces to send")
	flag.Parse()

	cc, err := grpc.NewClient(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *target, err)
	}
	defer cc.Close()
	client := ptraceotlp.NewGRPCClient(cc)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+*key)

	for tIdx := 0; tIdx < *traces; tIdx++ {
		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "seed-traces")
		ss := rs.ScopeSpans().AppendEmpty()

		var trID [16]byte
		// Distinct trace_id per iteration so listing reports each as a separate trace.
		trID[0] = byte(tIdx + 1)
		for i := 0; i < *spans; i++ {
			s := ss.Spans().AppendEmpty()
			s.SetName(fmt.Sprintf("op-%d", i))
			s.SetTraceID(pcommon.TraceID(trID))
			s.SetSpanID(pcommon.SpanID([8]byte{byte(tIdx + 1), byte(i + 1)}))
			now := time.Now()
			s.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
			s.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))
		}

		if _, err := client.Export(ctx, ptraceotlp.NewExportRequestFromTraces(td)); err != nil {
			log.Fatalf("export trace %d: %v", tIdx, err)
		}
	}
	log.Printf("seeded %d traces × %d spans → %s", *traces, *spans, *target)
}
