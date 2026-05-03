package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
	"github.com/cibersabueso/challengebeeyong/backend/internal/service"
)

const maxIdempotencyKeyLen = 256

// ReservationsHandler exposes reservation lifecycle endpoints.
type ReservationsHandler struct {
	reservations *service.ReservationService
}

// NewReservationsHandler wires a new ReservationsHandler.
func NewReservationsHandler(reservations *service.ReservationService) *ReservationsHandler {
	return &ReservationsHandler{reservations: reservations}
}

type createBody struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

// Create handles POST /reservations.
//
// Validation order (spec.md 6.1):
//  1. X-User-Id (handled by RequireUserID middleware before reaching here).
//  2. Idempotency-Key present and non-empty.
//  3. Idempotency-Key length and printable.
//  4. JSON body parseable.
//  5. item_id valid UUID v4.
//  6. quantity is a positive integer.
//  7. Idempotency check (handled inside service).
//  8. Item exists (handled inside service via AtomicDecrement disambiguation).
//  9. Atomic decrement (handled inside service).
func (h *ReservationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		WriteError(w, http.StatusBadRequest, domain.CodeMissingIdempotencyKey, "Idempotency-Key header is required", nil)
		return
	}

	if len(key) > maxIdempotencyKeyLen {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidIdempotencyKey, "Idempotency-Key exceeds 256 characters", nil)
		return
	}
	if !IsPrintableASCII(key) {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidIdempotencyKey, "Idempotency-Key contains non-printable characters", nil)
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidRequestBody, "could not read request body", nil)
		return
	}
	if len(rawBody) == 0 {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidRequestBody, "request body is empty", nil)
		return
	}

	var body createBody
	dec := json.NewDecoder(bytes.NewReader(rawBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidRequestBody, "request body is not valid json", nil)
		return
	}

	if !uuidV4Regex.MatchString(body.ItemID) {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidItemID, "item_id must be a valid UUID v4", nil)
		return
	}
	itemID, err := uuid.Parse(body.ItemID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidItemID, "item_id must be a valid UUID v4", nil)
		return
	}

	if body.Quantity <= 0 {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidQuantity, "quantity must be a positive integer", nil)
		return
	}

	result, err := h.reservations.Create(r.Context(), service.CreateRequest{
		IdempotencyKey: key,
		UserID:         userID,
		ItemID:         itemID,
		Quantity:       body.Quantity,
		RawPayload:     rawBody,
	})
	if err != nil {
		MapDomainError(w, err)
		return
	}

	WriteJSON(w, result.StatusCode, result.Reservation)
}

// Release handles DELETE /reservations/{id}.
func (h *ReservationsHandler) Release(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())

	idStr := chi.URLParam(r, "id")
	if !uuidV4Regex.MatchString(idStr) {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidReservationID, "reservation id must be a valid UUID v4", nil)
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidReservationID, "reservation id must be a valid UUID v4", nil)
		return
	}

	result, err := h.reservations.Release(r.Context(), id, userID)
	if err != nil {
		MapDomainError(w, err)
		return
	}

	switch result.Outcome {
	case service.ReleaseOutcomeReleased:
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":         "released",
			"reservation_id": result.ReservationID,
		})
	case service.ReleaseOutcomeAlreadyReleased:
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":         "already_released",
			"reservation_id": result.ReservationID,
		})
	case service.ReleaseOutcomeAlreadyExpired:
		MapDomainError(w, domain.ErrReservationExpired)
	case service.ReleaseOutcomeNotFoundOrForeign:
		MapDomainError(w, domain.ErrReservationNotFound)
	default:
		MapDomainError(w, errors.New("unexpected release outcome"))
	}
}

// ListMine handles GET /reservations.
func (h *ReservationsHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())

	out, err := h.reservations.ListByUser(r.Context(), userID)
	if err != nil {
		MapDomainError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, out)
}
