package service

import (
	"context"
	"fmt"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
	"github.com/cibersabueso/challengebeeyong/backend/internal/repository"
)

// ItemService exposes read operations over the inventory.
type ItemService struct {
	items *repository.ItemRepository
}

// NewItemService wires a new ItemService.
func NewItemService(items *repository.ItemRepository) *ItemService {
	return &ItemService{items: items}
}

// List returns the full inventory with computed available counts.
func (s *ItemService) List(ctx context.Context) ([]domain.Item, error) {
	out, err := s.items.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	return out, nil
}
