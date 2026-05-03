package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Cause classifies the reason behind a stock mutation log entry.
type Cause string

const (
	CauseReservationCreate  Cause = "reservation_create"
	CauseReservationRelease Cause = "reservation_release"
	CauseTTLExpired         Cause = "ttl_expired"
)

// LogStockMutation emits a structured log entry describing a stock mutation event.
// All mutations of items.reserved must call this helper to satisfy RNF-05.
func LogStockMutation(ctx context.Context, cause Cause, reservationID, itemID, userID uuid.UUID, delta int) {
	slog.InfoContext(ctx, "stock_mutation",
		"cause", string(cause),
		"reservation_id", reservationID.String(),
		"item_id", itemID.String(),
		"user_id", userID.String(),
		"delta", delta,
	)
}
