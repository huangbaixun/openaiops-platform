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
	DomainID          uuid.UUID // uuid.Nil when the tenant is ungrouped (NULL domain_id)
	Environment       string    // "" when unset (NULL environment)
}
