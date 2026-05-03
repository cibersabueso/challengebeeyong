package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
	"github.com/cibersabueso/challengebeeyong/backend/internal/repository"
)

// ReservationService orchestrates reservation lifecycle operations across repositories
// inside Postgres transactions. It owns the idempotency check, the atomic decrement,
// and the structured logging on stock mutations.
type ReservationService struct {
	pool         *pgxpool.Pool
	items        *repository.ItemRepository
	reservations *repository.ReservationRepository
	idempotency  *repository.IdempotencyRepository
	ttlSeconds   int
}

// NewReservationService wires a new ReservationService.
func NewReservationService(
	pool *pgxpool.Pool,
	items *repository.ItemRepository,
	reservations *repository.ReservationRepository,
	idempotency *repository.IdempotencyRepository,
	ttlSeconds int,
) *ReservationService {
	return &ReservationService{
		pool:         pool,
		items:        items,
		reservations: reservations,
		idempotency:  idempotency,
		ttlSeconds:   ttlSeconds,
	}
}

// CreateOutcome describes the result of Create from the caller's perspective.
type CreateOutcome string

const (
	OutcomeCreated          CreateOutcome = "created"
	OutcomeIdempotentReplay CreateOutcome = "idempotent_replay"
)

// CreateResult is the result of a Create call.
type CreateResult struct {
	Outcome     CreateOutcome
	Reservation *domain.Reservation
	StatusCode  int
}

// CreateRequest is the input for Create.
type CreateRequest struct {
	IdempotencyKey string
	UserID         uuid.UUID
	ItemID         uuid.UUID
	Quantity       int
	RawPayload     []byte
}

// Create reserves Quantity units of ItemID for UserID with idempotency guarantees.
//
// Flow inside ONE transaction:
//  1. Lookup idempotency key. Hit with same hash -> return cached. Hit with different hash -> 422 conflict.
//  2. Atomic decrement of items.reserved (UPDATE ... WHERE available >= qty).
//  3. Insert reservation row with TTL.
//  4. Persist idempotency record. On unique-violation -> re-lookup and return cached (concurrent race).
//  5. Commit. Log stock_mutation.
func (s *ReservationService) Create(ctx context.Context, req CreateRequest) (*CreateResult, error) {
	hash, err := CanonicalHash(req.RawPayload)
	if err != nil {
		return nil, fmt.Errorf("canonical hash: %w", err)
	}

	preExisting, err := s.idempotency.Lookup(ctx, s.pool, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency pre-lookup: %w", err)
	}
	if preExisting != nil {
		if preExisting.RequestHash != hash {
			return nil, domain.ErrIdempotencyConflict
		}
		return s.replayFromRecord(preExisting)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin create tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	item, err := s.items.AtomicDecrementAvailable(ctx, tx, req.ItemID, req.Quantity)
	if err != nil {
		return nil, err
	}

	res, err := s.reservations.Insert(ctx, tx, req.ItemID, req.UserID, req.Quantity, s.ttlSeconds)
	if err != nil {
		return nil, fmt.Errorf("insert reservation: %w", err)
	}

	body, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("marshal response body: %w", err)
	}
	resID := res.ID
	if err := s.idempotency.Persist(ctx, tx, req.IdempotencyKey, hash, &resID, 201, body); err != nil {
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			_ = tx.Rollback(ctx)
			winning, lerr := s.idempotency.Lookup(ctx, s.pool, req.IdempotencyKey)
			if lerr != nil {
				return nil, fmt.Errorf("re-lookup after conflict: %w", lerr)
			}
			if winning == nil {
				return nil, fmt.Errorf("inconsistent state: conflict without record")
			}
			if winning.RequestHash != hash {
				return nil, domain.ErrIdempotencyConflict
			}
			return s.replayFromRecord(winning)
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create: %w", err)
	}

	LogStockMutation(ctx, CauseReservationCreate, res.ID, item.ID, req.UserID, req.Quantity)

	return &CreateResult{
		Outcome:     OutcomeCreated,
		Reservation: res,
		StatusCode:  201,
	}, nil
}

func (s *ReservationService) replayFromRecord(rec *repository.IdempotencyRecord) (*CreateResult, error) {
	var res domain.Reservation
	if err := json.Unmarshal(rec.ResponseBody, &res); err != nil {
		return nil, fmt.Errorf("unmarshal cached body: %w", err)
	}
	return &CreateResult{
		Outcome:     OutcomeIdempotentReplay,
		Reservation: &res,
		StatusCode:  200,
	}, nil
}

// ReleaseOutcome describes the user-facing outcome of a release attempt.
type ReleaseOutcome string

const (
	ReleaseOutcomeReleased          ReleaseOutcome = "released"
	ReleaseOutcomeAlreadyReleased   ReleaseOutcome = "already_released"
	ReleaseOutcomeAlreadyExpired    ReleaseOutcome = "already_expired"
	ReleaseOutcomeNotFoundOrForeign ReleaseOutcome = "not_found_or_foreign"
)

// ReleaseResult is the result of a Release call.
type ReleaseResult struct {
	Outcome       ReleaseOutcome
	ReservationID uuid.UUID
	ReleasedAt    *string
}

// Release marks an active reservation as released and returns its quantity to the item pool.
// Idempotent: repeated calls return a stable outcome and the stock is returned exactly once.
func (s *ReservationService) Release(ctx context.Context, reservationID, userID uuid.UUID) (*ReleaseResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin release tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	outcome, qty, itemID, err := s.reservations.AtomicReleaseByOwner(ctx, tx, reservationID, userID)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case repository.OutcomeReleased:
		if err := s.items.ReturnStock(ctx, tx, itemID, qty); err != nil {
			return nil, fmt.Errorf("return stock on release: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit release: %w", err)
		}
		LogStockMutation(ctx, CauseReservationRelease, reservationID, itemID, userID, -qty)
		return &ReleaseResult{Outcome: ReleaseOutcomeReleased, ReservationID: reservationID}, nil

	case repository.OutcomeAlreadyReleased:
		return &ReleaseResult{Outcome: ReleaseOutcomeAlreadyReleased, ReservationID: reservationID}, nil

	case repository.OutcomeAlreadyExpired:
		return &ReleaseResult{Outcome: ReleaseOutcomeAlreadyExpired, ReservationID: reservationID}, nil

	case repository.OutcomeNotFoundOrForeign:
		return &ReleaseResult{Outcome: ReleaseOutcomeNotFoundOrForeign, ReservationID: reservationID}, nil

	default:
		return nil, fmt.Errorf("unexpected release outcome: %s", outcome)
	}
}

// ListByUser returns the active reservations of a user.
func (s *ReservationService) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Reservation, error) {
	out, err := s.reservations.ListActiveByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list reservations: %w", err)
	}
	return out, nil
}
