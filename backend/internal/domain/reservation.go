package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of a reservation.
type Status string

const (
	StatusActive   Status = "active"
	StatusReleased Status = "released"
	StatusExpired  Status = "expired"
)

// Reservation represents a temporary hold on N units of an item for a user.
type Reservation struct {
	ID         uuid.UUID  `json:"id"`
	ItemID     uuid.UUID  `json:"item_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Quantity   int        `json:"quantity"`
	Status     Status     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}
