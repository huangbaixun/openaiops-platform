package ingestshared

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// pgExecIface is the narrow subset of *sql.DB used by metering.
type pgExecIface interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type fakePGExec struct {
	inserts []struct {
		tid    uuid.UUID
		signal string
		count  int
	}
	failErr error
}

func (f *fakePGExec) ExecContext(_ context.Context, _ string, args ...any) (sql.Result, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	f.inserts = append(f.inserts, struct {
		tid    uuid.UUID
		signal string
		count  int
	}{args[0].(uuid.UUID), args[1].(string), args[2].(int)})
	return nil, nil
}

// meteringForTest is a test-only constructor that replaces the *sql.DB with a
// pgExecIface and does NOT start the background goroutine (so Drain can be
// called synchronously).
type meteringForTest struct {
	pg      pgExecIface
	metrics *BaseMetrics
	signal  string
	queue   chan MeteringEvent
}

func newMeteringForTest(pg pgExecIface, metrics *BaseMetrics, signal string, cap int) *meteringForTest {
	return &meteringForTest{
		pg:      pg,
		metrics: metrics,
		signal:  signal,
		queue:   make(chan MeteringEvent, cap),
	}
}

func (m *meteringForTest) Enqueue(ev MeteringEvent) {
	select {
	case m.queue <- ev:
	default:
		m.metrics.MeteringFailed.WithLabelValues(ev.SignalType, "queue_full").Inc()
	}
}

func (m *meteringForTest) Drain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case ev := <-m.queue:
			m.write(ctx, ev)
		default:
			return
		}
	}
}

func (m *meteringForTest) write(ctx context.Context, ev MeteringEvent) {
	_, err := m.pg.ExecContext(ctx,
		`INSERT INTO metering_events (tenant_id, signal_type, count, ts) VALUES ($1,$2,$3, now())`,
		ev.TenantID, ev.SignalType, ev.Count,
	)
	if err != nil {
		m.metrics.MeteringFailed.WithLabelValues(ev.SignalType, "pg_error").Inc()
	}
}

func TestMeteringShared_EnqueueDrain(t *testing.T) {
	pg := &fakePGExec{}
	metrics := NewBaseMetrics(prometheus.NewRegistry(), "trace")
	m := newMeteringForTest(pg, metrics, "trace", 4)

	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	m.Enqueue(MeteringEvent{TenantID: tid, SignalType: "trace", Count: 7})
	m.Drain(context.Background())

	if len(pg.inserts) != 1 {
		t.Fatalf("inserts = %d, want 1", len(pg.inserts))
	}
	if pg.inserts[0].count != 7 {
		t.Fatalf("count = %d, want 7", pg.inserts[0].count)
	}
	if pg.inserts[0].signal != "trace" {
		t.Fatalf("signal_type = %q, want trace", pg.inserts[0].signal)
	}
}

func TestMeteringShared_PGErrorIncrementsFailedCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewBaseMetrics(reg, "trace")
	m := newMeteringForTest(&fakePGExec{failErr: errors.New("pg down")}, metrics, "trace", 4)

	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	m.Enqueue(MeteringEvent{TenantID: tid, SignalType: "trace", Count: 5})
	m.Drain(context.Background())

	got := testCounterVecValue(t, metrics.MeteringFailed, "trace", "pg_error")
	if got != 1 {
		t.Fatalf("MeteringFailed{pg_error} = %v, want 1", got)
	}
}

func TestMeteringShared_QueueFullIncrementsFailedCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewBaseMetrics(reg, "trace")
	m := newMeteringForTest(&fakePGExec{}, metrics, "trace", 1)

	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	m.Enqueue(MeteringEvent{TenantID: tid, SignalType: "trace", Count: 1})
	m.Enqueue(MeteringEvent{TenantID: tid, SignalType: "trace", Count: 1})
	// First fills the buffer, second is dropped → MeteringFailed queue_full should increment.
	got := testCounterVecValue(t, metrics.MeteringFailed, "trace", "queue_full")
	if got != 1 {
		t.Fatalf("MeteringFailed{queue_full} = %v, want 1", got)
	}
}

// testCounterVecValue reads a CounterVec value for the given label values.
func testCounterVecValue(t *testing.T, cv *prometheus.CounterVec, lvs ...string) float64 {
	t.Helper()
	c, err := cv.GetMetricWithLabelValues(lvs...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	if m.Counter == nil {
		return 0
	}
	return *m.Counter.Value
}
