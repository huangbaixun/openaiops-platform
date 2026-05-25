package ingest

import (
	"context"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Consumer is the consumer.Traces impl wired into the OTLP receiver.
// Full pipeline (auth → spans → CH → metering) lands in Task 5.
// For T4 we accept and drop, so the receiver verifies wiring end-to-end.
type Consumer struct {
	// wiring fields added in Task 5
}

func NewConsumer() *Consumer { return &Consumer{} }

func (c *Consumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (c *Consumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	_ = ctx
	_ = td
	return nil
}
