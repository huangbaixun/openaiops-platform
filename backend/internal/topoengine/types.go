// Package topoengine is the background aggregator that derives
// topology_edges_v1 + service_stats_v1 from traces_v1 every 1 minute.
//
// Bucket discipline: a tick at T always processes ClosedBucketAt(T) =
// the bucket strictly before T's containing minute, so no in-flight
// ingest writes can land in a bucket being aggregated.
//
// Tenant trust: discovery reads the PG tenants table; per-tenant
// aggregation uses chquery.Conn under auth.WithTenant(...) (SQL filter
// + Row Policy + custom_tenant_id).
package topoengine

import (
	"database/sql"
	"time"

	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
)

// Config is engine tuning loaded from env via internal/config.
type Config struct {
	TickInterval      time.Duration // default 1m
	CatchupMax        time.Duration // default 1h — older gaps accepted as loss
	TenantConcurrency int           // default 4 — per-bucket errgroup parallelism
}

// DefaultConfig returns the same defaults that config.FromEnv() applies when
// TOPO_* env vars are unset. Used in tests that bypass env loading.
// IF YOU CHANGE A DEFAULT HERE, also change it in internal/config/config.go's
// FromEnv() TOPO_* block, and vice versa.
func DefaultConfig() Config {
	return Config{
		TickInterval:      time.Minute,
		CatchupMax:        time.Hour,
		TenantConcurrency: 4,
	}
}

// Deps is the set of long-lived clients the engine holds.
type Deps struct {
	CH    *chquery.Conn      // tenant-scoped CH access
	Admin *chquery.AdminConn // tenant-unaware admin queries
	PG    *sql.DB            // tenant discovery: SELECT id FROM tenants (PLATFORM-TOPO-1)
}

// Engine is the topo aggregation pipeline.
type Engine struct {
	cfg     Config
	deps    Deps
	metrics *Metrics
}

func New(cfg Config, deps Deps, metrics *Metrics) *Engine {
	return &Engine{cfg: cfg, deps: deps, metrics: metrics}
}
