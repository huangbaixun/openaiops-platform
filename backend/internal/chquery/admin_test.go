package chquery

import (
	"context"
	"testing"
	"time"
)

func TestAdminQueryKind_String(t *testing.T) {
	if got := AdminListTenants.String(); got != "AdminListTenants" {
		t.Fatalf("AdminListTenants.String() = %q, want AdminListTenants", got)
	}
	if got := AdminMaxBucket.String(); got != "AdminMaxBucket" {
		t.Fatalf("AdminMaxBucket.String() = %q, want AdminMaxBucket", got)
	}
}

func TestAdminQueryKind_SQL_NotEmpty(t *testing.T) {
	for _, k := range []AdminQueryKind{AdminListTenants, AdminMaxBucket} {
		if sql := k.sql(); sql == "" {
			t.Fatalf("AdminQueryKind(%v).sql() is empty", k)
		}
	}
}

// AdminConn is constructed via NewAdminConn(*Conn). Verify it does not
// panic when given a nil Conn — the constructor must not dereference c.
func TestNewAdminConn_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewAdminConn(nil) panicked: %v", r)
		}
	}()
	ac := NewAdminConn(nil)
	_ = ac
}

// AdminQuery must reject unknown AdminQueryKind values at runtime — guards
// against unsafe extension where a caller smuggles in a new constant value.
func TestAdminConn_AdminQuery_RejectsUnknownKind(t *testing.T) {
	ac := NewAdminConn(&Conn{}) // inner c is nil but unknown-kind check fires before driver
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ac.AdminQuery(ctx, AdminQueryKind(9999))
	if err == nil {
		t.Fatalf("AdminQuery with unknown kind should error, got nil")
	}
	want := "chquery: unknown AdminQueryKind"
	if got := err.Error(); !contains(got, want) {
		t.Fatalf("AdminQuery error = %q, want substring %q", got, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
