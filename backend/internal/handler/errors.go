package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
)

// ErrorResponse is the canonical envelope for every error returned by the API.
type ErrorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON serializes payload as JSON and writes it with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("encode response", "err", err)
	}
}

// WriteError writes an ErrorResponse with the given status and code.
func WriteError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	WriteJSON(w, status, ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	})
}

// MapDomainError translates a sentinel domain error to its HTTP representation.
// Unknown errors fall through to 500 with a generic INTERNAL_ERROR code.
func MapDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrOutOfStock):
		WriteError(w, http.StatusConflict, domain.CodeOutOfStock, err.Error(), nil)
	case errors.Is(err, domain.ErrIdempotencyConflict):
		WriteError(w, http.StatusUnprocessableEntity, domain.CodeIdempotencyConflict, err.Error(), nil)
	case errors.Is(err, domain.ErrItemNotFound):
		WriteError(w, http.StatusNotFound, domain.CodeItemNotFound, err.Error(), nil)
	case errors.Is(err, domain.ErrReservationNotFound):
		WriteError(w, http.StatusNotFound, domain.CodeReservationNotFound, err.Error(), nil)
	case errors.Is(err, domain.ErrReservationExpired):
		WriteError(w, http.StatusGone, domain.CodeReservationExpired, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidQuantity):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidQuantity, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidUserID):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidUserID, err.Error(), nil)
	case errors.Is(err, domain.ErrMissingIdempotencyKey):
		WriteError(w, http.StatusBadRequest, domain.CodeMissingIdempotencyKey, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidIdempotencyKey):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidIdempotencyKey, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidRequestBody):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidRequestBody, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidItemID):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidItemID, err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidReservationID):
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidReservationID, err.Error(), nil)
	default:
		slog.Error("unmapped error", "err", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}
