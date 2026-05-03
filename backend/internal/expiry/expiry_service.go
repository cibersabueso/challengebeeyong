package expiry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cibersabueso/challengebeeyong/backend/internal/repository"
)

// IdempotencyRetention is the TTL for entries in the idempotency_keys table.
// After this period, entries are eligible for purge by the maintenance pass.
const IdempotencyRetention = 24 * time.Hour

// Service runs the periodic maintenance pass: expire overdue reservations and
// purge stale idempotency records.
type Service struct {
	reservations *repository.ReservationRepository
	idempotency  *repository.IdempotencyRepository
	interval     time.Duration
}

// NewService wires a new expiry Service.
func NewService(reservations *repository.ReservationRepository, idempotency *repository.IdempotencyRepository, intervalSeconds int) *Service {
	return &Service{
		reservations: reservations,
		idempotency:  idempotency,
		interval:     time.Duration(intervalSeconds) * time.Second,
	}
}

// RunOnce executes a single maintenance pass: expire overdue reservations and
// purge old idempotency records. Errors are logged but not propagated, since
// the loop must keep running.
func (s *Service) RunOnce(ctx context.Context) {
	expired, err := s.reservations.ExpireBatch(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "expire batch failed", "err", err)
	} else if expired > 0 {
		slog.InfoContext(ctx, "reservations expired", "count", expired, "cause", "ttl_expired")
	}

	purged, err := s.idempotency.PurgeExpired(ctx, IdempotencyRetention)
	if err != nil {
		slog.ErrorContext(ctx, "purge idempotency failed", "err", err)
	} else if purged > 0 {
		slog.InfoContext(ctx, "idempotency keys purged", "count", purged)
	}
}

// Loop runs RunOnce on every tick until ctx is canceled.
func (s *Service) Loop(ctx context.Context) {
	if s.interval <= 0 {
		slog.WarnContext(ctx, "expiry interval is zero or negative; loop disabled")
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(context.Background(), "expiry loop stopped")
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// Bootstrap performs a single synchronous pass intended to clean up state left over
// from a previous run before the server begins accepting requests.
func (s *Service) Bootstrap(ctx context.Context) error {
	slog.InfoContext(ctx, "bootstrap expiry cleanup starting")
	s.RunOnce(ctx)
	slog.InfoContext(ctx, "bootstrap expiry cleanup done")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("bootstrap canceled: %w", err)
	}
	return nil
}
