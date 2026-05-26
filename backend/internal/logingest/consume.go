package logingest

import (
	"context"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
)

// LogConsumer is the consumer.Logs impl wired into the OTLP receiver.
// Pipeline (filled in by T5): Bearer extract → auth.Resolver → server-stamp
// tenant_id → logsToRows → chquery.Batch → metering enqueue.
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
	// Skeleton: T5 replaces with full auth+write+metering pipeline.
	_ = ctx
	_ = ld
	return nil
}
