package handler

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
)

type ctxKey string

const (
	ctxKeyUserID ctxKey = "user_id"
)

// uuidV4Regex matches strict UUID v4 (version=4, variant=8|9|a|b).
var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// RequireUserID extracts and validates the X-User-Id header as a strict UUID v4.
// On success, the parsed UUID is injected into the request context.
// On failure, responds 400 INVALID_USER_ID and does not call next.
func RequireUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("X-User-Id")
		if !uuidV4Regex.MatchString(raw) {
			WriteError(w, http.StatusBadRequest, domain.CodeInvalidUserID, "X-User-Id header must be a valid UUID v4", nil)
			return
		}
		parsed, err := uuid.Parse(raw)
		if err != nil {
			WriteError(w, http.StatusBadRequest, domain.CodeInvalidUserID, "X-User-Id header must be a valid UUID v4", nil)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUserID, parsed)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext returns the validated UUID v4 of the current request.
// Panics if RequireUserID middleware did not run; callers must always wire it.
func UserIDFromContext(ctx context.Context) uuid.UUID {
	v, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	if !ok {
		panic("user_id missing from context: RequireUserID middleware not wired")
	}
	return v
}

// IsPrintableASCII reports whether s contains only printable ASCII characters (0x20-0x7E).
func IsPrintableASCII(s string) bool {
	for _, c := range s {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}
