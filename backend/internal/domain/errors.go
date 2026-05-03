package domain

import "errors"

// Error codes returned by the API as machine-readable identifiers.
// They map 1-to-1 with the enum defined in specs/reservation/openapi.yaml.
const (
	CodeOutOfStock                 = "OUT_OF_STOCK"
	CodeIdempotencyConflict        = "IDEMPOTENCY_CONFLICT"
	CodeReservationExpired         = "RESERVATION_EXPIRED"
	CodeReservationNotFound        = "RESERVATION_NOT_FOUND"
	CodeReservationAlreadyReleased = "RESERVATION_ALREADY_RELEASED"
	CodeInvalidQuantity            = "INVALID_QUANTITY"
	CodeItemNotFound               = "ITEM_NOT_FOUND"
	CodeInvalidUserID              = "INVALID_USER_ID"
	CodeMissingIdempotencyKey      = "MISSING_IDEMPOTENCY_KEY"
	CodeInvalidIdempotencyKey      = "INVALID_IDEMPOTENCY_KEY"
	CodeInvalidRequestBody         = "INVALID_REQUEST_BODY"
	CodeInvalidItemID              = "INVALID_ITEM_ID"
	CodeInvalidReservationID       = "INVALID_RESERVATION_ID"
)

// Sentinel errors used across services and repositories to communicate
// well-defined business outcomes without leaking storage details.
var (
	ErrOutOfStock                 = errors.New("out of stock")
	ErrIdempotencyConflict        = errors.New("idempotency conflict")
	ErrReservationExpired         = errors.New("reservation expired")
	ErrReservationNotFound        = errors.New("reservation not found")
	ErrReservationAlreadyReleased = errors.New("reservation already released")
	ErrInvalidQuantity            = errors.New("invalid quantity")
	ErrItemNotFound               = errors.New("item not found")
	ErrInvalidUserID              = errors.New("invalid user id")
	ErrMissingIdempotencyKey      = errors.New("missing idempotency key")
	ErrInvalidIdempotencyKey      = errors.New("invalid idempotency key")
	ErrInvalidRequestBody         = errors.New("invalid request body")
	ErrInvalidItemID              = errors.New("invalid item id")
	ErrInvalidReservationID       = errors.New("invalid reservation id")
)
