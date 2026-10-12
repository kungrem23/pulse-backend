package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/kungrem23/pulse-backend/internal/security"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

type TokenVerifier interface {
	Verify(rawToken string, expectedType security.TokenType) (security.Claims, error)
}

type userIDKey struct{}

// RequireUser admits only requests with a valid user access JWT. Refresh
// tokens and admin principals are rejected.
func RequireUser(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if verifier == nil {
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
				unauthorized(w)
				return
			}
			claims, err := verifier.Verify(token, security.AccessToken)
			if err != nil || claims.Principal != security.UserPrincipal {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, claims.Subject)))
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
