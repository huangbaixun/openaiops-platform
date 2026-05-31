//go:build integration

package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

func TestAnnotationsPruner_RunPrunesThenStops(t *testing.T) {
	ctx := context.Background()
	db, t1, _ := pgForAnnotations(t)
	repo := query.NewAnnotationsRepo(db)
	tid := uuid.MustParse(t1)

	_, err := db.ExecContext(ctx, `
		INSERT INTO annotations (tenant_id, target_type, target_id, kind, payload, ts, idempotency_key, created_at)
		VALUES ($1,'service','checkout','ai_rca','{}'::jsonb, now(), 'old', now() - interval '40 days')
	`, tid)
	require.NoError(t, err)

	pruner := query.NewAnnotationsPruner(repo, 30, time.Hour) // long interval: only the immediate prune runs
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { pruner.Run(runCtx); close(done) }()

	require.Eventually(t, func() bool {
		var k *string
		_ = db.QueryRowContext(ctx, `SELECT idempotency_key FROM annotations WHERE target_id='checkout'`).Scan(&k)
		return k == nil
	}, 3*time.Second, 50*time.Millisecond, "initial prune should null the old key")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}
