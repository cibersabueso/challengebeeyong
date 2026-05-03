package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
)

// ReservationRepository encapsulates all SQL operations against the reservations table.
type ReservationRepository struct {
	pool *pgxpool.Pool
}

// NewReservationRepository builds a new ReservationRepository bound to the given pool.
func NewReservationRepository(pool *pgxpool.Pool) *ReservationRepository {
	return &ReservationRepository{pool: pool}
}

const queryInsertReservation = `
INSERT INTO reservations (id, item_id, user_id, quantity, status, expires_at, created_at)
VALUES ($1, $2, $3, $4, 'active', NOW() + ($5::int * interval '1 second'), NOW())
RETURNING id, item_id, user_id, quantity, status, expires_at, created_at, released_at
`

// Insert persists a new active reservation with the given TTL in seconds.
func (r *ReservationRepository) Insert(ctx context.Context, exec Executor, itemID, userID uuid.UUID, quantity, ttlSeconds int) (*domain.Reservation, error) {
	id := uuid.New()
	row := exec.QueryRow(ctx, queryInsertReservation, id, itemID, userID, quantity, ttlSeconds)

	var res domain.Reservation
	err := row.Scan(&res.ID, &res.ItemID, &res.UserID, &res.Quantity, &res.Status, &res.ExpiresAt, &res.CreatedAt, &res.ReleasedAt)
	if err != nil {
		return nil, fmt.Errorf("insert reservation: %w", err)
	}
	return &res, nil
}

const queryFindByID = `
SELECT id, item_id, user_id, quantity, status, expires_at, created_at, released_at
  FROM reservations
 WHERE id = $1
`

// FindByID retrieves a reservation by id, regardless of owner or status.
func (r *ReservationRepository) FindByID(ctx context.Context, exec Executor, id uuid.UUID) (*domain.Reservation, error) {
	row := exec.QueryRow(ctx, queryFindByID, id)
	var res domain.Reservation
	err := row.Scan(&res.ID, &res.ItemID, &res.UserID, &res.Quantity, &res.Status, &res.ExpiresAt, &res.CreatedAt, &res.ReleasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrReservationNotFound
		}
		return nil, fmt.Errorf("find reservation: %w", err)
	}
	return &res, nil
}

const queryAtomicRelease = `
UPDATE reservations
   SET status = 'released',
       released_at = NOW()
 WHERE id = $1
   AND user_id = $2
   AND status = 'active'
RETURNING quantity, item_id
`

// ReleaseOutcome describes the result of an attempted release.
type ReleaseOutcome string

const (
	OutcomeReleased          ReleaseOutcome = "released"
	OutcomeAlreadyReleased   ReleaseOutcome = "already_released"
	OutcomeAlreadyExpired    ReleaseOutcome = "already_expired"
	OutcomeNotFoundOrForeign ReleaseOutcome = "not_found_or_foreign"
)

// AtomicReleaseByOwner attempts to release an active reservation owned by userID.
// Returns the outcome plus (quantity, itemID) when the release succeeded so the caller
// can decrement the items.reserved counter inside the same transaction.
func (r *ReservationRepository) AtomicReleaseByOwner(ctx context.Context, exec Executor, id, userID uuid.UUID) (ReleaseOutcome, int, uuid.UUID, error) {
	var qty int
	var itemID uuid.UUID

	row := exec.QueryRow(ctx, queryAtomicRelease, id, userID)
	err := row.Scan(&qty, &itemID)
	if err == nil {
		return OutcomeReleased, qty, itemID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, uuid.Nil, fmt.Errorf("atomic release: %w", err)
	}

	res, ferr := r.FindByID(ctx, exec, id)
	if ferr != nil {
		if errors.Is(ferr, domain.ErrReservationNotFound) {
			return OutcomeNotFoundOrForeign, 0, uuid.Nil, nil
		}
		return "", 0, uuid.Nil, ferr
	}
	if res.UserID != userID {
		return OutcomeNotFoundOrForeign, 0, uuid.Nil, nil
	}
	switch res.Status {
	case domain.StatusReleased:
		return OutcomeAlreadyReleased, 0, uuid.Nil, nil
	case domain.StatusExpired:
		return OutcomeAlreadyExpired, 0, uuid.Nil, nil
	default:
		return "", 0, uuid.Nil, fmt.Errorf("unexpected reservation status after no-op release: %s", res.Status)
	}
}

const queryListByUserActive = `
SELECT id, item_id, user_id, quantity, status, expires_at, created_at, released_at
  FROM reservations
 WHERE user_id = $1
   AND status = 'active'
   AND expires_at > NOW()
 ORDER BY created_at DESC
`

// ListActiveByUser returns all active reservations for userID whose TTL has not elapsed yet.
func (r *ReservationRepository) ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]domain.Reservation, error) {
	rows, err := r.pool.Query(ctx, queryListByUserActive, userID)
	if err != nil {
		return nil, fmt.Errorf("list active reservations: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Reservation, 0, 8)
	for rows.Next() {
		var res domain.Reservation
		if err := rows.Scan(&res.ID, &res.ItemID, &res.UserID, &res.Quantity, &res.Status, &res.ExpiresAt, &res.CreatedAt, &res.ReleasedAt); err != nil {
			return nil, fmt.Errorf("scan reservation: %w", err)
		}
		out = append(out, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return out, nil
}

const queryExpireBatch = `
WITH expired AS (
    UPDATE reservations
       SET status = 'expired'
     WHERE status = 'active'
       AND expires_at <= NOW()
    RETURNING id, item_id, quantity
)
UPDATE items i
   SET reserved = i.reserved - e.quantity
  FROM expired e
 WHERE i.id = e.item_id
RETURNING e.id
`

// ExpireBatch marks all overdue active reservations as expired and atomically returns
// their stock to the pool. Returns the count of reservations expired in this pass.
func (r *ReservationRepository) ExpireBatch(ctx context.Context) (int, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin expiry tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, queryExpireBatch)
	if err != nil {
		return 0, fmt.Errorf("expire batch: %w", err)
	}
	count := 0
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired id: %w", err)
		}
		count++
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit expiry: %w", err)
	}
	return count, nil
}

// Pool exposes the underlying pool for service-layer transaction management.
func (r *ReservationRepository) Pool() *pgxpool.Pool {
	return r.pool
}

// Now is exported as a hook for tests that may want to override time perception.
// Not currently used; reserved for future use without changing call sites.
var Now = time.Now
