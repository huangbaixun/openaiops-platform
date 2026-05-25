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

func NewMetrics() *Metrics {
	return &Metrics{
		AuthMissing: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ingester_auth_missing_total",
		}),
		AuthInvalid: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ingester_auth_invalid_total",
		}),
		SpansAccepted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_accepted_total",
		}, []string{"tenant_id", "service"}),
		SpansRejected: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ingester_spans_rejected_total",
		}, []string{"reason"}),
		MeteringFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ingester_metering_failed_total",
		}),
		BatchDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ingester_batch_duration_seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"outcome"}),
	}
}
