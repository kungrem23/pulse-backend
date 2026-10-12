package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

type Authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (string, error)
}

type userIDKey struct{}

// RequireUser admits only requests with a valid user access JWT of an existing,
// unblocked account. Refresh tokens and admin principals are rejected.
func RequireUser(authenticator Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
				unauthorized(w)
				return
			}
			userID, err := authenticator.Authenticate(r.Context(), token)
			switch {
			case err == nil:
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
			case errors.Is(err, appauth.ErrUserBlocked):
				httpx.Error(w, http.StatusForbidden, "user is blocked")
			case errors.Is(err, appauth.ErrInvalidAccessToken):
				unauthorized(w)
			default:
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
			}
		})
	}
}

// UserID returns the authenticated user's ID set by RequireUser.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok && id != ""
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer`)
	httpx.Error(w, http.StatusUnauthorized, "authentication required")
}
