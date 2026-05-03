package domain

import (
	"time"

	"github.com/google/uuid"
)

// Item represents an inventory item with fixed total stock.
type Item struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Total     int       `json:"total"`
	Reserved  int       `json:"reserved"`
	Available int       `json:"available"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}
