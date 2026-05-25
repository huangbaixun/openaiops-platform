package ingest

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

const insertTracesV1Stmt = `INSERT INTO traces_v1 (
    tenant_id, trace_id, span_id, parent_span_id, service, operation,
    ts, duration, status, span_kind, resource_attributes, attributes
) VALUES`

// Consumer is the consumer.Traces impl wired into the OTLP receiver.
// Pipeline: Bearer extract → auth.Resolver → server-stamp tenant_id →
// spans→rows → chquery.Batch → metering enqueue.
type Consumer struct {
	resolver auth.Resolver
	ch       *chquery.Conn
	metering *Metering // nil-safe; Task 6 wires real impl
	metrics  *Metrics
}

func NewConsumer(resolver auth.Resolver, ch *chquery.Conn, metering *Metering, metrics *Metrics) *Consumer {
	return &Consumer{resolver: resolver, ch: ch, metering: metering, metrics: metrics}
}

func (c *Consumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (c *Consumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	start := time.Now()

	bearer, err := extractBearer(ctx)
	if err != nil {
		c.metrics.AuthMissing.Inc()
		c.metrics.BatchDuration.WithLabelValues("auth_missing").Observe(time.Since(start).Seconds())
		return status.Error(codes.Unauthenticated, "missing bearer")
	}
	_, tn, err := c.resolver.ResolveBearer(ctx, bearer)
	if err != nil {
		c.metrics.AuthInvalid.Inc()
		c.metrics.BatchDuration.WithLabelValues("auth_invalid").Observe(time.Since(start).Seconds())
		return status.Error(codes.Unauthenticated, "invalid bearer")
	}
	ctx = auth.WithTenant(ctx, tn.ID, tn.Name)

	rows := spansToRows(td)
	if len(rows) == 0 {
		c.metrics.BatchDuration.WithLabelValues("empty").Observe(time.Since(start).Seconds())
		return nil
	}

	batch, err := c.ch.PrepareBatch(ctx, insertTracesV1Stmt)
	if err != nil {
		c.metrics.SpansRejected.WithLabelValues("ch_prepare_failed").Add(float64(len(rows)))
		c.metrics.BatchDuration.WithLabelValues("ch_prepare_failed").Observe(time.Since(start).Seconds())
		return status.Error(codes.Internal, fmt.Sprintf("prepare: %v", err))
	}
	tidStr := tn.ID.String()
	for _, r := range rows {
		if err := batch.Append(
			tidStr, r.TraceID, r.SpanID, r.ParentSpanID, r.Service, r.Operation,
			r.Ts, r.DurationNs, r.Status, r.SpanKind, r.ResourceAttributes, r.Attributes,
		); err != nil {
			_ = batch.Abort()
			c.metrics.SpansRejected.WithLabelValues("ch_append_failed").Add(float64(len(rows)))
			c.metrics.BatchDuration.WithLabelValues("ch_append_failed").Observe(time.Since(start).Seconds())
			return status.Error(codes.Internal, fmt.Sprintf("append: %v", err))
		}
	}
	if err := batch.Send(); err != nil {
		c.metrics.SpansRejected.WithLabelValues("ch_send_failed").Add(float64(len(rows)))
		c.metrics.BatchDuration.WithLabelValues("ch_send_failed").Observe(time.Since(start).Seconds())
		return status.Error(codes.Internal, fmt.Sprintf("send: %v", err))
	}
	c.metrics.SpansAccepted.WithLabelValues(tidStr, firstService(rows)).Add(float64(len(rows)))

	if c.metering != nil {
		c.metering.Enqueue(tn.ID, len(rows))
	}

	c.metrics.BatchDuration.WithLabelValues("ok").Observe(time.Since(start).Seconds())
	return nil
}

func firstService(rows []SpanRow) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].Service
}
