package ingestshared

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MeteringEvent is one row destined for PG metering_events.
type MeteringEvent struct {
	TenantID   uuid.UUID
	SignalType string // "trace" | "log" (extend in SLICE-4 for "metric")
	Count      int
}

// Metering is a best-effort PG sink. Enqueue returns immediately; failures are
// counted and logged. Drain blocks until the queue is empty or ctx done.
type Metering struct {
	db      *sql.DB
	metrics *BaseMetrics
	signal  string // "trace" | "log" — used in metric label
	queue   chan MeteringEvent
	wg      sync.WaitGroup
	closed  chan struct{}
}

// NewMetering starts an async metering writer. signal is used for counter
// labels ("trace" or "log"). Caller must call Drain then Close on shutdown.
func NewMetering(db *sql.DB, metrics *BaseMetrics, signal string) *Metering {
	m := &Metering{
		db:      db,
		metrics: metrics,
		signal:  signal,
		queue:   make(chan MeteringEvent, 1024),
		closed:  make(chan struct{}),
	}
	m.wg.Add(1)
	go m.run()
	return m
}

func (m *Metering) Enqueue(ev MeteringEvent) {
	select {
	case m.queue <- ev:
	default:
		m.metrics.MeteringFailed.WithLabelValues(ev.SignalType, "queue_full").Inc()
		slog.Warn("metering queue full; dropping event",
			"tenant_id", ev.TenantID, "signal", ev.SignalType, "count", ev.Count)
	}
}

func (m *Metering) run() {
	defer m.wg.Done()
	for ev := range m.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := m.db.ExecContext(ctx,
			`INSERT INTO metering_events (tenant_id, signal_type, count, ts) VALUES ($1,$2,$3, now())`,
			ev.TenantID, ev.SignalType, ev.Count,
		)
		cancel()
		if err != nil {
			m.metrics.MeteringFailed.WithLabelValues(ev.SignalType, "pg_error").Inc()
			slog.Error("metering write failed", "err", err, "signal_type", ev.SignalType)
		}
	}
	close(m.closed)
}

// Drain processes any pending events, bounded by ctx, then stops the worker.
// Call Drain before Close on shutdown.
func (m *Metering) Drain(ctx context.Context) {
	close(m.queue)
	select {
	case <-m.closed:
	case <-ctx.Done():
	}
}

// Close is idempotent; Drain is the real shutdown path.
func (m *Metering) Close() {
	// Nothing to do — Drain closes the queue and waits for the goroutine.
}
