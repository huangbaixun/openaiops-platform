package chquery

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// adminSentinelTenantID is the value injected as custom_tenant_id when
// AdminConn runs a whitelisted query. CH Row Policies on business tables
// reference getSetting('custom_tenant_id') and will error with code 115
// (UNKNOWN_SETTING) if the setting is wholly absent. Injecting a sentinel
// avoids that crash. The policy will then evaluate `tenant_id = ''` which
// admits zero rows for any normal tenant — by design.
//
// In production, topo-engine connects under a CH user that the
// `tenant_isolation_*` Row Policies do NOT apply to (operator-managed
// deploy config). That user genuinely sees all tenants. The sentinel
// covers the test path and any deploy slip where the policy still applies.
const adminSentinelTenantID = ""

// AdminQueryKind enumerates the small, fixed set of admin queries that
// topo-engine (or other internal services) may run *without* a tenant_id
// in context. Each kind maps to a single SQL constant — callers cannot
// pass free-form SQL. Add a new kind here only with a code review.
type AdminQueryKind int

const (
	// AdminListTenants selects distinct tenant_id values from traces_v1
	// observed since the provided since-time arg (DateTime64). Used by
	// topo-engine.activeTenants() to discover work units per tick.
	AdminListTenants AdminQueryKind = iota + 1

	// AdminMaxBucket selects max(ts_bucket) from topology_edges_v1 FINAL
	// for the tenant_id given as the only arg (String). Used by
	// topo-engine.lastCompletedBucket() to seed Catchup.
	AdminMaxBucket
)

// String returns the constant's name for logging / errors.
func (k AdminQueryKind) String() string {
	switch k {
	case AdminListTenants:
		return "AdminListTenants"
	case AdminMaxBucket:
		return "AdminMaxBucket"
	default:
		return fmt.Sprintf("AdminQueryKind(%d)", int(k))
	}
}

// sql returns the SQL template associated with this kind. Returns "" for
// unknown kinds so callers can reject before reaching the driver.
func (k AdminQueryKind) sql() string {
	switch k {
	case AdminListTenants:
		return `SELECT DISTINCT tenant_id FROM traces_v1 WHERE ts >= ?`
	case AdminMaxBucket:
		return `SELECT max(ts_bucket) FROM topology_edges_v1 FINAL WHERE tenant_id = ?`
	}
	return ""
}

// AdminConn is a tenant-unaware wrapper that bypasses MustTenantScope.
// Construction is restricted to packages allowed by deploy/lint-no-bare-ch.sh —
// currently only internal/topoengine/. Direct construction elsewhere fails CI.
//
// AdminConn injects an empty-sentinel `custom_tenant_id` setting (see
// `adminSentinelTenantID`) rather than a real tenant ID, so it bypasses
// MustTenantScope but still satisfies the CH UNKNOWN_SETTING constraint.
// In production this requires the CH user to be exempted from
// `tenant_isolation_*` Row Policies (operator concern, tracked in
// progress.json known_drift).
type AdminConn struct {
	c *Conn
}

// NewAdminConn wraps an existing chquery.Conn for admin use. The caller
// retains ownership of the inner Conn; Close() on AdminConn is a no-op
// because AdminConn never owns the underlying driver connection.
func NewAdminConn(c *Conn) *AdminConn { return &AdminConn{c: c} }

// adminCtx returns ctx with the sentinel custom_tenant_id session setting
// applied. See adminSentinelTenantID for why this is needed.
func adminCtx(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"custom_tenant_id": clickhouse.CustomSetting{Value: adminSentinelTenantID},
	}))
}

// AdminQuery runs the SQL associated with kind, passing args through.
// Returns an error for any kind not in the whitelist.
func (a *AdminConn) AdminQuery(ctx context.Context, kind AdminQueryKind, args ...any) (driver.Rows, error) {
	sql := kind.sql()
	if sql == "" {
		return nil, fmt.Errorf("chquery: unknown AdminQueryKind %s", kind)
	}
	return a.c.c.Query(adminCtx(ctx), sql, args...)
}

// AdminQueryRow runs a whitelisted SQL kind that returns a single row.
// Unknown kinds surface as a Scan-time error via errRow (the driver.Row
// contract has no construction-time error channel).
func (a *AdminConn) AdminQueryRow(ctx context.Context, kind AdminQueryKind, args ...any) driver.Row {
	sql := kind.sql()
	if sql == "" {
		return errRow{err: fmt.Errorf("chquery: unknown AdminQueryKind %s", kind)}
	}
	return a.c.c.QueryRow(adminCtx(ctx), sql, args...)
}

// errRow implements driver.Row to surface a constructor-time error on Scan.
// Matches clickhouse-go v2.30.0 driver.Row: Err / Scan / ScanStruct.
type errRow struct{ err error }

func (e errRow) Err() error           { return e.err }
func (e errRow) Scan(...any) error    { return e.err }
func (e errRow) ScanStruct(any) error { return e.err }
