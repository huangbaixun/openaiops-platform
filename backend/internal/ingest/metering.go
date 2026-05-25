package ingest

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// pgExec is the narrow interface the metering writer needs.
// *sql.DB satisfies it; tests pass a fake.
type pgExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type meteringEvent struct {
	tid   uuid.UUID
	count int
}

// Metering writes per-batch usage events to PG asynchronously. Best-effort:
// CH commit returns OK to SDK regardless of PG metering outcome.
// Failure modes:
//   - queue full → drop event + MeteringFailed.Inc()
//   - PG insert error → log + MeteringFailed.Inc()
type Metering struct {
	pg      pgExec
	metrics *Metrics
	ch      chan meteringEvent
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// Default channel capacity. Tunable if ingester throughput justifies.
const meteringQueueCap = 1024

func NewMetering(pg pgExec, metrics *Metrics) *Metering {
	m := newMeteringWithCap(pg, metrics, meteringQueueCap)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.wg.Add(1)
	go m.loop(ctx)
	return m
}

// newMeteringWithCap constructs a Metering with the channel configured but
// does NOT start the consumer goroutine. Test-only path used to exercise
// queue-full behavior deterministically.
func newMeteringWithCap(pg pgExec, metrics *Metrics, cap int) *Metering {
	return &Metering{
		pg:      pg,
		metrics: metrics,
		ch:      make(chan meteringEvent, cap),
	}
}

func (m *Metering) Enqueue(tid uuid.UUID, count int) {
	select {
	case m.ch <- meteringEvent{tid: tid, count: count}:
	default:
		m.metrics.MeteringFailed.Inc()
		slog.Warn("metering queue full; dropping event", "tenant_id", tid, "count", count)
	}
}

func (m *Metering) loop(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-m.ch:
			m.write(ev)
		}
	}
}

func (m *Metering) write(ev meteringEvent) {
	_, err := m.pg.ExecContext(context.Background(),
		`INSERT INTO metering_events (tenant_id, signal_type, count) VALUES ($1, $2, $3)`,
		ev.tid, "trace", ev.count)
	if err != nil {
		m.metrics.MeteringFailed.Inc()
		slog.Error("metering insert failed", "tenant_id", ev.tid, "err", err)
	}
}

// Drain processes pending events synchronously. Safe to call after Close.
// Used in tests and during shutdown when we want best-effort flush.
func (m *Metering) Drain() {
	for {
		select {
		case ev := <-m.ch:
			m.write(ev)
		default:
			return
		}
	}
}

// Close stops the consumer goroutine. Does NOT drain — call Drain() first if
// you want pending events flushed.
func (m *Metering) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}
