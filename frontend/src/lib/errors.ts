export type ApiErrorCode =
  | "OUT_OF_STOCK"
  | "IDEMPOTENCY_CONFLICT"
  | "RESERVATION_EXPIRED"
  | "RESERVATION_NOT_FOUND"
  | "RESERVATION_ALREADY_RELEASED"
  | "INVALID_QUANTITY"
  | "ITEM_NOT_FOUND"
  | "INVALID_USER_ID"
  | "MISSING_IDEMPOTENCY_KEY"
  | "INVALID_IDEMPOTENCY_KEY"
  | "INVALID_REQUEST_BODY"
  | "INVALID_ITEM_ID"
  | "INVALID_RESERVATION_ID"
  | "INTERNAL_ERROR"
  | "NETWORK_ERROR";

export interface ApiErrorPayload {
  code: ApiErrorCode;
  message: string;
  details?: Record<string, unknown>;
}

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  readonly details?: Record<string, unknown>;

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message);
    this.name = "ApiError";
    this.code = payload.code;
    this.status = status;
    this.details = payload.details;
  }
}

const USER_FACING_MESSAGES: Partial<Record<ApiErrorCode, string>> = {
  OUT_OF_STOCK: "Item Taken — this item was just reserved by another user.",
  RESERVATION_EXPIRED: "This reservation already expired.",
  RESERVATION_NOT_FOUND: "Reservation not found.",
  IDEMPOTENCY_CONFLICT: "Conflicting request payload.",
  INVALID_QUANTITY: "Invalid quantity selected.",
  NETWORK_ERROR: "Network error. Please retry.",
};

export function userMessageFor(err: unknown): string {
  if (err instanceof ApiError) {
    return USER_FACING_MESSAGES[err.code] ?? err.message;
  }
  if (err instanceof Error) return err.message;
  return "Unexpected error.";
}
