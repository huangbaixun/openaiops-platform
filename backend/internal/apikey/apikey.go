package apikey

import (
	"time"

	"github.com/google/uuid"
)

type ApiKey struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	Name       string
	HashedKey  string
	Scope      string
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

func (k *ApiKey) IsActive() bool {
	return k.RevokedAt == nil
}
