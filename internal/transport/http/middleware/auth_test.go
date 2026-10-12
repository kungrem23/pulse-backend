package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
)

type authenticatorStub map[string]error

func (s authenticatorStub) Authenticate(_ context.Context, token string) (string, error) {
	if err, ok := s[token]; ok {
		if err != nil {
			return "", err
		}
		return "user-1", nil
	}
	return "", appauth.ErrInvalidAccessToken
}

func TestRequireUser(t *testing.T) {
	authenticator := authenticatorStub{
		"valid":   nil,
		"blocked": appauth.ErrUserBlocked,
		"broken":  errors.New("database unavailable"),
	}
	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantUserID string
	}{
		{"valid token", "Bearer valid", http.StatusOK, "user-1"},
		{"lowercase scheme", "bearer valid", http.StatusOK, "user-1"},
		{"missing header", "", http.StatusUnauthorized, ""},
		{"empty token", "Bearer ", http.StatusUnauthorized, ""},
		{"wrong scheme", "Basic valid", http.StatusUnauthorized, ""},
		{"invalid token", "Bearer other", http.StatusUnauthorized, ""},
		{"blocked user", "Bearer blocked", http.StatusForbidden, ""},
		{"storage failure", "Bearer broken", http.StatusInternalServerError, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotUserID string
			handler := RequireUser(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestRequireUserWithoutAuthenticatorFailsClosed(t *testing.T) {
	called := false
	handler := RequireUser(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if called || response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without reaching the handler, got %d", response.Code)
	}
}
