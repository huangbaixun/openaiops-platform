// SLICE-3 T15 — seed-topology: emit a hot-r.o.d.-style 4-service shape
// (frontend → checkout → payment internal edges + checkout → redis Client
// external edge) so topo-engine derives 4 topology_edges_v1 rows on its
// next 1-min tick.
//
// Mirrors backend/cmd/seed-traces/main.go: directly constructs pdata.Traces
// + uses ptraceotlp.GRPCClient (no OTel SDK). Honors INGESTER_OTLP_GRPC_HOST_PORT
// for local SignOz collision override.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	port := envOr("INGESTER_OTLP_GRPC_HOST_PORT", "4317")
	target := flag.String("target", "127.0.0.1:"+port, "ingester OTLP/gRPC addr")
	apiKey := flag.String("api-key", envOr("API_KEY", "test-key-acme"), "Bearer key (resolves to a tenant)")
	count := flag.Int("count", 5, "trace iterations to emit")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "API_KEY (or --api-key) required")
		os.Exit(2)
	}

	cc, err := grpc.NewClient(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *target, err)
	}
	defer cc.Close()
	client := ptraceotlp.NewGRPCClient(cc)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+*apiKey)

	// Build one ExportRequest with 4 ResourceSpans (one per service). All spans
	// share a single trace_id per iteration so parent/child links across services
	// are consistent. Each iteration uses a fresh trace_id so listing reports
	// independent traces.
	for tIdx := 0; tIdx < *count; tIdx++ {
		td := ptrace.NewTraces()

		var trID [16]byte
		trID[0] = 0x01
		trID[15] = byte(tIdx + 1)

		// Span IDs (unique per role).
		var frontendSpan, checkoutSpan, paymentSpan, redisSpan [8]byte
		frontendSpan[0] = 0xA0
		frontendSpan[7] = byte(tIdx + 1)
		checkoutSpan[0] = 0xA1
		checkoutSpan[7] = byte(tIdx + 1)
		paymentSpan[0] = 0xA2
		paymentSpan[7] = byte(tIdx + 1)
		redisSpan[0] = 0xA3
		redisSpan[7] = byte(tIdx + 1)

		now := time.Now()
		startNano := pcommon.NewTimestampFromTime(now)
		endNano := pcommon.NewTimestampFromTime(now.Add(50 * time.Millisecond))

		// 1) frontend: SERVER GET /checkout (root)
		emitRoot := td.ResourceSpans().AppendEmpty()
		emitRoot.Resource().Attributes().PutStr("service.name", "frontend")
		fs := emitRoot.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		fs.SetName("GET /checkout")
		fs.SetTraceID(pcommon.TraceID(trID))
		fs.SetSpanID(pcommon.SpanID(frontendSpan))
		fs.SetKind(ptrace.SpanKindServer)
		fs.SetStartTimestamp(startNano)
		fs.SetEndTimestamp(endNano)

		// 2) checkout: SERVER POST /charge (child of frontend) → derives edge frontend→checkout
		emitCheckout := td.ResourceSpans().AppendEmpty()
		emitCheckout.Resource().Attributes().PutStr("service.name", "checkout")
		cs := emitCheckout.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		cs.SetName("POST /charge")
		cs.SetTraceID(pcommon.TraceID(trID))
		cs.SetSpanID(pcommon.SpanID(checkoutSpan))
		cs.SetParentSpanID(pcommon.SpanID(frontendSpan))
		cs.SetKind(ptrace.SpanKindServer)
		cs.SetStartTimestamp(startNano)
		cs.SetEndTimestamp(endNano)

		// 3) payment: SERVER auth (child of checkout) → derives edge checkout→payment
		emitPayment := td.ResourceSpans().AppendEmpty()
		emitPayment.Resource().Attributes().PutStr("service.name", "payment")
		ps := emitPayment.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		ps.SetName("auth")
		ps.SetTraceID(pcommon.TraceID(trID))
		ps.SetSpanID(pcommon.SpanID(paymentSpan))
		ps.SetParentSpanID(pcommon.SpanID(checkoutSpan))
		ps.SetKind(ptrace.SpanKindServer)
		ps.SetStartTimestamp(startNano)
		ps.SetEndTimestamp(endNano)

		// 4) checkout: CLIENT redis GET with db.system=redis (child of checkout)
		//    → derives external edge checkout→redis (callee_kind=external)
		emitRedis := td.ResourceSpans().AppendEmpty()
		emitRedis.Resource().Attributes().PutStr("service.name", "checkout")
		rs := emitRedis.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		rs.SetName("redis GET")
		rs.SetTraceID(pcommon.TraceID(trID))
		rs.SetSpanID(pcommon.SpanID(redisSpan))
		rs.SetParentSpanID(pcommon.SpanID(checkoutSpan))
		rs.SetKind(ptrace.SpanKindClient)
		rs.Attributes().PutStr("db.system", "redis")
		rs.SetStartTimestamp(startNano)
		rs.SetEndTimestamp(endNano)

		if _, err := client.Export(ctx, ptraceotlp.NewExportRequestFromTraces(td)); err != nil {
			log.Fatalf("export iter %d: %v", tIdx, err)
		}
	}

	fmt.Printf("seed-topology: emitted %d traces × 4 spans (frontend→checkout→payment + checkout→redis client) → %s\n",
		*count, *target)
	fmt.Println("Wait ~120s for topo-engine to process the closed minute bucket, then open https://localhost/topology")
}
