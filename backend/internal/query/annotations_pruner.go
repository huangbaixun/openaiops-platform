package query

import (
	"context"
	"log/slog"
	"time"
)

// AnnotationsPruner periodically nulls expired annotation idempotency keys so the
// partial unique index uq_annotations_idem stays bounded (PLATFORM-ANN-1 / D9).
type AnnotationsPruner struct {
	repo     *AnnotationsRepo
	days     int
	interval time.Duration
}

func NewAnnotationsPruner(repo *AnnotationsRepo, days int, interval time.Duration) *AnnotationsPruner {
	return &AnnotationsPruner{repo: repo, days: days, interval: interval}
}

// Run prunes once immediately, then every interval until ctx is cancelled. A prune
// error is logged and retried on the next tick — it never aborts the loop.
func (p *AnnotationsPruner) Run(ctx context.Context) {
	p.pruneOnce(ctx)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.pruneOnce(ctx)
		}
	}
}

func (p *AnnotationsPruner) pruneOnce(ctx context.Context) {
	n, err := p.repo.PruneIdempotencyKeys(ctx, p.days)
	if err != nil {
		slog.Warn("annotations pruner: prune failed (will retry next tick)", "err", err)
		return
	}
	if n > 0 {
		slog.Info("annotations pruner: nulled expired idempotency keys", "rows", n, "retention_days", p.days)
	}
}
