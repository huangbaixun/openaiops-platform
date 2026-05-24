package tenant

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID                uuid.UUID
	Name              string
	Plan              string
	RateLimitPerMin   int
	DataRetentionDays int
	CreatedAt         time.Time
}
