package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	AuthMissing    prometheus.Counter
	AuthInvalid    prometheus.Counter
	SpansAccepted  *prometheus.CounterVec
	SpansRejected  *prometheus.CounterVec
	MeteringFailed prometheus.Counter
	BatchDuration  *prometheus.HistogramVec
}

// NewMetrics constructs Prometheus collectors against the provided Registerer.
// Pass prometheus.DefaultRegisterer in production (so the admin /metrics handler
// picks them up). Pass prometheus.NewRegistry() in tests to avoid
// "duplicate metrics collector registration" panics when the test binary
// constructs multiple Consumers.
func NewMetrics(r prometheus.Registerer) *Metrics {
	f := promauto.With(r)
	return &Metrics{
		AuthMissing: f.NewCounter(prometheus.CounterOpts{
			Name: "ingester_auth_missing_total",
		}),
		AuthInvalid: f.NewCounter(prometheus.CounterOpts{
			Name: "ingester_auth_invalid_total",
		}),
		SpansAccepted: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_accepted_total",
		}, []string{"tenant_id", "service"}),
		SpansRejected: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_rejected_total",
		}, []string{"reason"}),
		MeteringFailed: f.NewCounter(prometheus.CounterOpts{
			Name: "ingester_metering_failed_total",
		}),
		BatchDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ingester_batch_duration_seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"outcome"}),
	}
}
