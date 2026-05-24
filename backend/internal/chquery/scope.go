// Package chquery enforces multi-tenant safety on every ClickHouse access.
// See ADR-0001 §3.3 + ADR-0003.
package chquery

// MustTenantScope and Conn are defined in subsequent tasks. This file exists
// so the package compiles before Task 2 lands the helper.
