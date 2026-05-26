package logingest

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
)

const insertLogsV1Stmt = `INSERT INTO logs_v1 (
    tenant_id, ts, observed_ts, service, severity_text, severity_number,
    body, trace_id, span_id, trace_flags, resource_attributes, attributes
) VALUES`

// LogConsumer is the consumer.Logs impl wired into the OTLP receiver.
// Pipeline: Bearer extract → auth.Resolver → server-stamp tenant_id →
// logsToRows → chquery.Batch → metering enqueue.
type LogConsumer struct {
	resolver auth.Resolver
	ch       *chquery.Conn
	metering *ingestshared.Metering
	metrics  *ingestshared.BaseMetrics
}

func NewLogConsumer(resolver auth.Resolver, ch *chquery.Conn, metering *ingestshared.Metering, metrics *ingestshared.BaseMetrics) *LogConsumer {
	return &LogConsumer{resolver: resolver, ch: ch, metering: metering, metrics: metrics}
}

func (c *LogConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (c *LogConsumer) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	bearer, err := ingestshared.ExtractBearer(ctx)
	if err != nil {
		c.metrics.AuthMissing.WithLabelValues("log").Inc()
		return status.Error(codes.Unauthenticated, "missing bearer")
	}
	_, tn, err := c.resolver.ResolveBearer(ctx, bearer)
	if err != nil {
		c.metrics.AuthInvalid.WithLabelValues("log").Inc()
		return status.Error(codes.Unauthenticated, "invalid bearer")
	}
	ctx = auth.WithTenant(ctx, tn.ID, tn.Name)

	rows := logsToRows(ld)
	if len(rows) == 0 {
		return nil
	}

	batch, err := c.ch.PrepareBatch(ctx, insertLogsV1Stmt)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("prepare: %v", err))
	}
	tidStr := tn.ID.String()
	for _, r := range rows {
		if err := batch.Append(
			tidStr, r.Ts, r.ObservedTs, r.Service, r.SeverityText, r.SeverityNumber,
			r.Body, r.TraceID, r.SpanID, r.TraceFlags, r.ResourceAttributes, r.Attributes,
		); err != nil {
			_ = batch.Abort()
			return status.Error(codes.Internal, fmt.Sprintf("append: %v", err))
		}
	}
	if err := batch.Send(); err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("send: %v", err))
	}

	if c.metering != nil {
		c.metering.Enqueue(ingestshared.MeteringEvent{
			TenantID:   tn.ID,
			SignalType: "log",
			Count:      len(rows),
		})
	}
	return nil
}
