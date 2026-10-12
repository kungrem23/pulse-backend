package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/security"
)

func TestRequireUser(t *testing.T) {
	tokens, err := security.NewTokenIssuer(security.TokenIssuerConfig{
		Secret:          "test-secret-with-at-least-thirty-two-bytes",
		Issuer:          "pulse-test",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, _ := tokens.Issue("user-1", security.UserPrincipal)
	admin, _ := tokens.Issue("admin-1", security.AdminPrincipal)

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantUserID string
	}{
		{"user access token", "Bearer " + user.AccessToken, http.StatusOK, "user-1"},
		{"lowercase scheme", "bearer " + user.AccessToken, http.StatusOK, "user-1"},
		{"missing header", "", http.StatusUnauthorized, ""},
		{"empty token", "Bearer ", http.StatusUnauthorized, ""},
		{"refresh token", "Bearer " + user.RefreshToken, http.StatusUnauthorized, ""},
		{"admin access token", "Bearer " + admin.AccessToken, http.StatusUnauthorized, ""},
		{"garbage", "Bearer not-a-jwt", http.StatusUnauthorized, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotUserID string
			handler := RequireUser(tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUserID, _ = UserID(r.Context())
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus || gotUserID != test.wantUserID {
				t.Fatalf("expected %d/%q, got %d/%q", test.wantStatus, test.wantUserID, response.Code, gotUserID)
			}
			if test.wantStatus == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("401 must carry a WWW-Authenticate challenge")
			}
		})
	}
}
