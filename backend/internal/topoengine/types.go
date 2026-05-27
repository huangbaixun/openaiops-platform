// Package topoengine is the background aggregator that derives
// topology_edges_v1 + service_stats_v1 from traces_v1 every 1 minute.
//
// Bucket discipline: a tick at T always processes ClosedBucketAt(T) =
// the bucket strictly before T's containing minute, so no in-flight
// ingest writes can land in a bucket being aggregated.
//
// Tenant trust: discovery uses chquery.AdminConn (whitelisted SQL only).
// Per-tenant aggregation uses chquery.Conn under auth.WithTenant(...),
// which provides the three-layer tenant safety (SQL filter + Row Policy
// + custom_tenant_id session setting).
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
	PG    *sql.DB            // reserved for future idempotency state
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
