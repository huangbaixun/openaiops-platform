package topoengine

import "github.com/prometheus/client_golang/prometheus"

// Metrics is the topo-engine Prometheus counter/gauge/histogram set.
// Pass prometheus.DefaultRegisterer in production; prometheus.NewRegistry()
// in tests.
type Metrics struct {
	TickTotal         *prometheus.CounterVec
	TickFailedTotal   prometheus.Counter
	TenantFailedTotal *prometheus.CounterVec
	TenantsProcessed  prometheus.Counter
	EdgesWritten      *prometheus.CounterVec
	ServicesWritten   *prometheus.CounterVec
	BucketLagSeconds  prometheus.Gauge
	PassDuration      *prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		TickTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "topo_engine_tick_total",
			Help: "Tick attempts by outcome (success|partial|failure)",
		}, []string{"outcome"}),
		TickFailedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "topo_engine_tick_failed_total",
			Help: "Tick attempts that failed wholesale (no tenants succeeded)",
		}),
		TenantFailedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "topo_engine_tenant_failed_total",
			Help: "Per-tenant aggregation failures in a tick",
		}, []string{"tenant_id"}),
		TenantsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "topo_engine_tenants_processed_total",
			Help: "Per-tenant aggregation successes across all ticks",
		}),
		EdgesWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "topo_engine_edges_written_total",
			Help: "Edge rows written to topology_edges_v1 per tenant",
		}, []string{"tenant_id"}),
		ServicesWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "topo_engine_services_written_total",
			Help: "Service rows written to service_stats_v1 per tenant",
		}, []string{"tenant_id"}),
		BucketLagSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "topo_engine_bucket_lag_seconds",
			Help: "now() - last completed bucket end (seconds)",
		}),
		PassDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "topo_engine_pass_duration_seconds",
			Help:    "Duration of a single pass (discover|edges|services) in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"pass"}),
	}
	reg.MustRegister(
		m.TickTotal,
		m.TickFailedTotal,
		m.TenantFailedTotal,
		m.TenantsProcessed,
		m.EdgesWritten,
		m.ServicesWritten,
		m.BucketLagSeconds,
		m.PassDuration,
	)
	return m
}
