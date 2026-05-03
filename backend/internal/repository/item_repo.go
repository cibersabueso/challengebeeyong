package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
)

// ItemRepository encapsulates all SQL operations against the items table.
type ItemRepository struct {
	pool *pgxpool.Pool
}

// NewItemRepository builds a new ItemRepository bound to the given pool.
func NewItemRepository(pool *pgxpool.Pool) *ItemRepository {
	return &ItemRepository{pool: pool}
}

const queryListItems = `
SELECT id, name, total, reserved, total - reserved AS available, created_at
  FROM items
 ORDER BY name ASC
`

// ListAll returns every item with computed available stock (total - reserved).
func (r *ItemRepository) ListAll(ctx context.Context) ([]domain.Item, error) {
	rows, err := r.pool.Query(ctx, queryListItems)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Item, 0, 16)
	for rows.Next() {
		var it domain.Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Total, &it.Reserved, &it.Available, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return out, nil
}

const queryAtomicDecrement = `
UPDATE items
   SET reserved = reserved + $1
 WHERE id = $2
   AND (total - reserved) >= $1
RETURNING id, name, total, reserved, total - reserved AS available, created_at
`

// AtomicDecrementAvailable atomically reserves N units of an item.
// Returns ErrOutOfStock if available stock is insufficient.
// Returns ErrItemNotFound if the item does not exist.
// Accepts an executor (pool or transaction) so it can participate in larger transactions.
func (r *ItemRepository) AtomicDecrementAvailable(ctx context.Context, exec Executor, itemID uuid.UUID, quantity int) (*domain.Item, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive: %w", domain.ErrInvalidQuantity)
	}

	row := exec.QueryRow(ctx, queryAtomicDecrement, quantity, itemID)

	var it domain.Item
	err := row.Scan(&it.ID, &it.Name, &it.Total, &it.Reserved, &it.Available, &it.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			exists, existsErr := r.itemExists(ctx, exec, itemID)
			if existsErr != nil {
				return nil, fmt.Errorf("disambiguate decrement failure: %w", existsErr)
			}
			if !exists {
				return nil, domain.ErrItemNotFound
			}
			return nil, domain.ErrOutOfStock
		}
		return nil, fmt.Errorf("atomic decrement: %w", err)
	}
	return &it, nil
}

const queryItemExists = `SELECT 1 FROM items WHERE id = $1`

func (r *ItemRepository) itemExists(ctx context.Context, exec Executor, itemID uuid.UUID) (bool, error) {
	var n int
	err := exec.QueryRow(ctx, queryItemExists, itemID).Scan(&n)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("item exists check: %w", err)
	}
	return true, nil
}

const queryReturnStock = `UPDATE items SET reserved = reserved - $1 WHERE id = $2`

// ReturnStock returns N units of an item to the pool. Used by release and expiry flows.
func (r *ItemRepository) ReturnStock(ctx context.Context, exec Executor, itemID uuid.UUID, quantity int) error {
	tag, err := exec.Exec(ctx, queryReturnStock, quantity, itemID)
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrItemNotFound
	}
	return nil
}
