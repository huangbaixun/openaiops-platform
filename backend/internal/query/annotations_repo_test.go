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

func TestAnnotationsRepo_PruneIdempotencyKeys(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)

	tid := query.MustUUID(t1)
	_, err := db.ExecContext(ctx, `
		INSERT INTO annotations (tenant_id, target_type, target_id, kind, payload, ts, idempotency_key, created_at)
		VALUES ($1,'service','checkout','ai_rca','{}'::jsonb, now(), 'old-key',    now() - interval '40 days'),
		       ($1,'service','payment', 'ai_rca','{}'::jsonb, now(), 'recent-key', now())
	`, tid)
	require.NoError(t, err)

	n, err := repo.PruneIdempotencyKeys(ctx, 30)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "only the 40-day-old key is pruned")

	var oldKey, recentKey *string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT idempotency_key FROM annotations WHERE target_id='checkout'`).Scan(&oldKey))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT idempotency_key FROM annotations WHERE target_id='payment'`).Scan(&recentKey))
	assert.Nil(t, oldKey, "old key nulled")
	require.NotNil(t, recentKey)
	assert.Equal(t, "recent-key", *recentKey, "recent key kept")

	var cnt int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM annotations WHERE tenant_id=$1`, tid).Scan(&cnt))
	assert.Equal(t, 2, cnt, "content preserved")

	// dedupe lapses for the freed key: reusing 'old-key' creates a NEW row.
	_, createdOld, err := repo.Insert(ctx, tid,
		query.AnnotationInput{
			TargetType: "service", TargetID: "checkout", Kind: "ai_rca",
			Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
		},
		"old-key")
	require.NoError(t, err)
	assert.True(t, createdOld, "freed key reusable -> new row")

	// recent key still dedupes.
	_, createdRecent, err := repo.Insert(ctx, tid,
		query.AnnotationInput{
			TargetType: "service", TargetID: "payment", Kind: "ai_rca",
			Payload: json.RawMessage(`{}`), TS: time.Now().UTC(),
		},
		"recent-key")
	require.NoError(t, err)
	assert.False(t, createdRecent, "live key still dedupes -> no new row")
}
