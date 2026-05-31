package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Annotation is one stored annotation row.
type Annotation struct {
	ID         uuid.UUID       `json:"id"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	TS         time.Time       `json:"ts"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AnnotationInput is the writable subset (tenant comes from ctx, never the body).
type AnnotationInput struct {
	TargetType string
	TargetID   string
	Kind       string
	Payload    json.RawMessage
	TS         time.Time
}

// AnnotationsRepo is a PG-backed store. Unlike the CH repos in this package it
// takes *sql.DB; annotations are low-volume relational metadata (see spec
// docs/specs/2026-05-29-platform-ask-2-annotations-design.md §1).
type AnnotationsRepo struct{ db *sql.DB }

func NewAnnotationsRepo(db *sql.DB) *AnnotationsRepo { return &AnnotationsRepo{db: db} }

// MustUUID is a tiny helper used by tests and callers that already validated input.
func MustUUID(s string) uuid.UUID { return uuid.MustParse(s) }

// Insert writes an annotation scoped to tenantID. When idemKey != "" it dedupes
// on (tenant_id, idempotency_key): a repeated key returns the existing id with
// created=false. tenantID always wins over any body-supplied tenant.
func (r *AnnotationsRepo) Insert(ctx context.Context, tenantID uuid.UUID, in AnnotationInput, idemKey string) (uuid.UUID, bool, error) {
	const ins = `
		INSERT INTO annotations(tenant_id, target_type, target_id, kind, payload, ts, idempotency_key)
		VALUES($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		ON CONFLICT (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING id`
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, ins,
		tenantID, in.TargetType, in.TargetID, in.Kind, []byte(in.Payload), in.TS, idemKey,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, err
	}
	// We only reach here when INSERT ... DO NOTHING fired, which the partial
	// unique index guarantees requires a non-empty idempotency_key. Guard the
	// invariant explicitly so a future direct caller with an empty key can't
	// silently 500 via an ErrNoRows from the wrong-shaped fallback query.
	if idemKey == "" {
		return uuid.Nil, false, fmt.Errorf("annotations: ON CONFLICT fired with empty idempotency key (invariant violation)")
	}
	// Conflict on a non-empty idempotency key: return the existing row.
	const sel = `SELECT id FROM annotations WHERE tenant_id = $1 AND idempotency_key = $2`
	if err := r.db.QueryRowContext(ctx, sel, tenantID, idemKey).Scan(&id); err != nil {
		return uuid.Nil, false, err
	}
	return id, false, nil
}

// PruneIdempotencyKeys nulls idempotency_key for annotations whose created_at is older
// than `days` days, returning the number of rows affected. Maintenance op — intentionally
// tenant-UNSCOPED (retention runs across all tenants); it only clears keys, never reads or
// exposes annotation content. Nulling frees the row from the partial unique index
// uq_annotations_idem (WHERE idempotency_key IS NOT NULL), bounding it. PLATFORM-ANN-1 / D9.
func (r *AnnotationsRepo) PruneIdempotencyKeys(ctx context.Context, days int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE annotations
		   SET idempotency_key = NULL
		 WHERE idempotency_key IS NOT NULL
		   AND created_at < now() - make_interval(days => $1)
	`, days)
	if err != nil {
		return 0, fmt.Errorf("annotations: prune idempotency keys: %w", err)
	}
	return res.RowsAffected()
}

// List returns annotations for tenantID + targetType, optionally narrowed to a
// single targetID, newest first.
func (r *AnnotationsRepo) List(ctx context.Context, tenantID uuid.UUID, targetType, targetID string, limit int) ([]Annotation, error) {
	q := `
		SELECT id, target_type, target_id, kind, payload, ts, created_at
		FROM annotations
		WHERE tenant_id = $1 AND target_type = $2`
	args := []any{tenantID, targetType}
	if targetID != "" {
		q += ` AND target_id = $3`
		args = append(args, targetID)
	}
	q += ` ORDER BY ts DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Annotation{}
	for rows.Next() {
		var a Annotation
		var payload []byte
		if err := rows.Scan(&a.ID, &a.TargetType, &a.TargetID, &a.Kind, &payload, &a.TS, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Payload = json.RawMessage(payload)
		out = append(out, a)
	}
	return out, rows.Err()
}
