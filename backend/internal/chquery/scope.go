// Package chquery enforces multi-tenant safety on every ClickHouse access.
// See ADR-0001 §3.3 + ADR-0003.
package chquery

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
)

var (
	selectShape = regexp.MustCompile(`(?i)\btenant_id\s*=\s*\?`)
	insertShape = regexp.MustCompile(`(?i)INSERT\s+INTO\s+\w+\s*\(\s*tenant_id\s*,`)
)

// MustTenantScope validates that the query carries a tenant_id placeholder
// and returns the same query with tenant_id (as string) prepended to args.
// Panics if:
//   - ctx has no tenant_id (programmer error — auth middleware should have set it)
//   - query is a SELECT/UPDATE/DELETE missing "tenant_id = ?"
//   - query is an INSERT missing "(tenant_id," as first column
//
// tenant_id is converted to its canonical string form because CH stores it as
// LowCardinality(String) per ADR-0001 §3.3, not UUID.
//
// See ADR-0001 §3.3, ADR-0003.
func MustTenantScope(ctx context.Context, query string, args ...any) (string, []any) {
	tid, err := auth.TenantID(ctx)
	if err != nil {
		panic(fmt.Errorf("chquery: ctx has no tenant_id (auth middleware did not run?): %w", err))
	}
	if !hasTenantPlaceholder(query) {
		panic(fmt.Errorf("chquery: query missing tenant_id placeholder: %q", query))
	}
	out := make([]any, 0, len(args)+1)
	out = append(out, tid.String())
	out = append(out, args...)
	return query, out
}

func hasTenantPlaceholder(q string) bool {
	trimmed := strings.TrimLeft(q, " \t\n\r")
	upperPrefix := strings.ToUpper(trimmed)
	if strings.HasPrefix(upperPrefix, "INSERT") {
		return insertShape.MatchString(q)
	}
	return selectShape.MatchString(q)
}
