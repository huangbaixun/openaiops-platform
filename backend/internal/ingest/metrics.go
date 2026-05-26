package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
)

// Metrics holds trace-ingester-specific counters plus a reference to the
// shared base counters (auth + metering). consume.go calls Base.AuthMissing
// etc. for the shared signals; SpansAccepted / SpansRejected / BatchDuration
// remain trace-only.
type Metrics struct {
	Base          *ingestshared.BaseMetrics
	SpansAccepted *prometheus.CounterVec
	SpansRejected *prometheus.CounterVec
	BatchDuration *prometheus.HistogramVec
}

// NewMetrics constructs trace-specific Prometheus collectors against r, and
// records the shared base metrics (already registered by the caller via
// ingestshared.NewBaseMetrics).
// Pass prometheus.DefaultRegisterer in production (so the admin /metrics
// handler picks them up). Pass prometheus.NewRegistry() in tests to avoid
// "duplicate metrics collector registration" panics.
func NewMetrics(r prometheus.Registerer, base *ingestshared.BaseMetrics) *Metrics {
	f := promauto.With(r)
	return &Metrics{
		Base: base,
		SpansAccepted: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_accepted_total",
		}, []string{"tenant_id", "service"}),
		SpansRejected: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_rejected_total",
		}, []string{"reason"}),
		BatchDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ingester_batch_duration_seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"outcome"}),
	}
}
