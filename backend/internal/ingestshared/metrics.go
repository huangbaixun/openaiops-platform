package ingestshared

import "github.com/prometheus/client_golang/prometheus"

// BaseMetrics holds counters shared between trace + log ingesters.
// signal label value: "trace" | "log".
type BaseMetrics struct {
	AuthMissing    *prometheus.CounterVec // labels: signal
	AuthInvalid    *prometheus.CounterVec // labels: signal
	MeteringFailed *prometheus.CounterVec // labels: signal, reason
}

// NewBaseMetrics constructs and registers the shared counter set.
// signal is used as the counter name prefix (e.g. "trace" → "trace_ingester_*_total").
// Pass prometheus.DefaultRegisterer in production; prometheus.NewRegistry() in tests.
func NewBaseMetrics(reg prometheus.Registerer, signal string) *BaseMetrics {
	m := &BaseMetrics{
		AuthMissing: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: signal + "_ingester_auth_missing_total",
			Help: "Bearer header was missing on incoming OTLP request",
		}, []string{"signal"}),
		AuthInvalid: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: signal + "_ingester_auth_invalid_total",
			Help: "Bearer header was present but did not resolve to a tenant",
		}, []string{"signal"}),
		MeteringFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: signal + "_ingester_metering_failed_total",
			Help: "Best-effort PG metering write failed (CH commit still acknowledged to SDK)",
		}, []string{"signal", "reason"}),
	}
	reg.MustRegister(m.AuthMissing, m.AuthInvalid, m.MeteringFailed)
	return m
}
