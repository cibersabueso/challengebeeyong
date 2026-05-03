package handler

import (
	"net/http"

	"github.com/cibersabueso/challengebeeyong/backend/internal/service"
)

// ItemsHandler exposes the inventory read endpoint.
type ItemsHandler struct {
	items *service.ItemService
}

// NewItemsHandler wires a new ItemsHandler.
func NewItemsHandler(items *service.ItemService) *ItemsHandler {
	return &ItemsHandler{items: items}
}

// List handles GET /items.
func (h *ItemsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.List(r.Context())
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, items)
}
