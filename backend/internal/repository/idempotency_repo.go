package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
)

// IdempotencyRecord represents the cached response for a previously seen Idempotency-Key.
type IdempotencyRecord struct {
	Key            string
	RequestHash    string
	ReservationID  *uuid.UUID
	ResponseStatus int
	ResponseBody   json.RawMessage
	CreatedAt      time.Time
}

// IdempotencyRepository persists and retrieves idempotency keys with their cached responses.
type IdempotencyRepository struct {
	pool *pgxpool.Pool
}

// NewIdempotencyRepository builds a new IdempotencyRepository bound to the given pool.
func NewIdempotencyRepository(pool *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

const queryLookupIdempotency = `
SELECT key, request_hash, reservation_id, response_status, response_body, created_at
  FROM idempotency_keys
 WHERE key = $1
`

// Lookup returns the cached idempotency record for key, or (nil, nil) on miss.
func (r *IdempotencyRepository) Lookup(ctx context.Context, exec Executor, key string) (*IdempotencyRecord, error) {
	row := exec.QueryRow(ctx, queryLookupIdempotency, key)
	var rec IdempotencyRecord
	err := row.Scan(&rec.Key, &rec.RequestHash, &rec.ReservationID, &rec.ResponseStatus, &rec.ResponseBody, &rec.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup idempotency key: %w", err)
	}
	return &rec, nil
}

const queryInsertIdempotency = `
INSERT INTO idempotency_keys (key, request_hash, reservation_id, response_status, response_body)
VALUES ($1, $2, $3, $4, $5)
`

// Persist stores a new idempotency record. Returns ErrIdempotencyConflict when the key
// already exists (caller should re-Lookup to retrieve the original response).
func (r *IdempotencyRepository) Persist(ctx context.Context, exec Executor, key, requestHash string, reservationID *uuid.UUID, status int, body json.RawMessage) error {
	_, err := exec.Exec(ctx, queryInsertIdempotency, key, requestHash, reservationID, status, body)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrIdempotencyConflict
		}
		return fmt.Errorf("persist idempotency key: %w", err)
	}
	return nil
}

const queryPurgeIdempotency = `DELETE FROM idempotency_keys WHERE created_at < NOW() - $1::interval`

// PurgeExpired removes idempotency records older than retention. Used by the maintenance goroutine.
func (r *IdempotencyRepository) PurgeExpired(ctx context.Context, retention time.Duration) (int64, error) {
	interval := fmt.Sprintf("%d seconds", int64(retention.Seconds()))
	tag, err := r.pool.Exec(ctx, queryPurgeIdempotency, interval)
	if err != nil {
		return 0, fmt.Errorf("purge idempotency: %w", err)
	}
	return tag.RowsAffected(), nil
}
