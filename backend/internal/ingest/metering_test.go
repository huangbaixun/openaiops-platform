package ingest

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fakePG struct {
	inserts []struct {
		tid   uuid.UUID
		count int
	}
}

func (f *fakePG) ExecContext(_ context.Context, _ string, args ...any) (sql.Result, error) {
	f.inserts = append(f.inserts, struct {
		tid   uuid.UUID
		count int
	}{args[0].(uuid.UUID), args[2].(int)})
	return nil, nil
}

func TestMetering_EnqueueDrain(t *testing.T) {
	pg := &fakePG{}
	m := newMeteringWithCap(pg, NewMetrics(prometheus.NewRegistry()), 4)
	// No Close() — no loop goroutine to cancel.

	m.Enqueue(uuid.MustParse("11111111-1111-1111-1111-111111111111"), 7)
	m.Drain(context.Background())

	if len(pg.inserts) != 1 {
		t.Fatalf("inserts = %d, want 1", len(pg.inserts))
	}
	if pg.inserts[0].count != 7 {
		t.Fatalf("count = %d, want 7", pg.inserts[0].count)
	}
}

type fakePGErr struct{}

func (fakePGErr) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, errors.New("pg down")
}

func TestMetering_PGErrorIncrementsFailedCounter(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	// Use the no-goroutine constructor so Drain is the only consumer.
	m := newMeteringWithCap(fakePGErr{}, metrics, 4)
	// No Close() needed — no goroutine to wait on.

	m.Enqueue(uuid.MustParse("11111111-1111-1111-1111-111111111111"), 5)
	m.Drain(context.Background())

	if got := testCounterValue(t, metrics.MeteringFailed); got != 1 {
		t.Fatalf("MeteringFailed = %v, want 1", got)
	}
}

func TestMetering_QueueFullIncrementsFailedCounter(t *testing.T) {
	pg := &fakePG{}
	metrics := NewMetrics(prometheus.NewRegistry())
	// Build a Metering with size-1 buffer to force a full queue quickly.
	m := newMeteringWithCap(pg, metrics, 1)
	defer m.Close()
	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	// Don't let the consumer goroutine drain; we don't start it. newMeteringWithCap
	// returns a Metering with the channel set up but no loop running.
	m.Enqueue(tid, 1)
	m.Enqueue(tid, 1)
	// First fills the buffer, second is dropped → MeteringFailed should increment.
	got := testCounterValue(t, metrics.MeteringFailed)
	if got != 1 {
		t.Fatalf("MeteringFailed = %v, want 1", got)
	}
}

// testCounterValue reads the current value of a Counter via Prometheus's dto.
func testCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	if m.Counter == nil {
		return 0
	}
	return *m.Counter.Value
}
