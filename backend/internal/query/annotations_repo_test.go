//go:build integration

package query_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

// pgForAnnotations opens the PG fixture (started in TestMain), applies
// migrations, truncates, and seeds two tenants.
func pgForAnnotations(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	db, err := sql.Open("pgx", annotationsPGDSN)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_ = goose.SetDialect("postgres")
	require.NoError(t, goose.Up(db, "../../migrations"))
	_, err = db.ExecContext(context.Background(),
		"TRUNCATE annotations, api_keys, tenants RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	var t1, t2 string
	require.NoError(t, db.QueryRow("INSERT INTO tenants(name) VALUES('acme') RETURNING id").Scan(&t1))
	require.NoError(t, db.QueryRow("INSERT INTO tenants(name) VALUES('beta') RETURNING id").Scan(&t2))
	return db, t1, t2
}

func TestAnnotationsRepo_InsertAndList(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := query.MustUUID(t1)

	in := query.AnnotationInput{
		TargetType: "service", TargetID: "checkout", Kind: "ai_rca",
		Payload: json.RawMessage(`{"summary":"db slow"}`), TS: time.Now().UTC(),
	}
	id, created, err := repo.Insert(ctx, tid, in, "")
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, "00000000-0000-0000-0000-000000000000", id.String())

	got, err := repo.List(ctx, tid, "service", "checkout", 100)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ai_rca", got[0].Kind)
}

func TestAnnotationsRepo_IdempotencyReturnsExisting(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := query.MustUUID(t1)
	in := query.AnnotationInput{
		TargetType: "trace", TargetID: "abc123", Kind: "ai_rca",
		Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
	}
	id1, created1, err := repo.Insert(ctx, tid, in, "key-1")
	require.NoError(t, err)
	assert.True(t, created1)
	id2, created2, err := repo.Insert(ctx, tid, in, "key-1")
	require.NoError(t, err)
	assert.False(t, created2, "second insert with same key must be a dedupe hit")
	assert.Equal(t, id1, id2, "dedupe must return the same annotation id")

	got, err := repo.List(ctx, tid, "trace", "abc123", 100)
	require.NoError(t, err)
	assert.Len(t, got, 1, "only one row despite two inserts")
}

func TestAnnotationsRepo_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	db, t1, t2 := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	_, _, err := repo.Insert(ctx, query.MustUUID(t1), query.AnnotationInput{
		TargetType: "service", TargetID: "checkout", Kind: "ai_rca",
		Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
	}, "")
	require.NoError(t, err)

	got, err := repo.List(ctx, query.MustUUID(t2), "service", "checkout", 100)
	require.NoError(t, err)
	assert.Empty(t, got, "tenant B must not see tenant A's annotation")
}
