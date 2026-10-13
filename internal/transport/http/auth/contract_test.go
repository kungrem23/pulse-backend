package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

func TestAuthResponsesMatchOpenAPI(t *testing.T) {
	spec := contracttest.Load(t)
	session := appauth.Session{AccessToken: "access", RefreshToken: "refresh", AccessExpiresAt: time.Now().UTC().Add(time.Minute), RefreshExpiresAt: time.Now().UTC().Add(time.Hour)}
	for _, test := range []struct {
		path, body  string
		service     *authServiceStub
		contentType string
	}{
		{"/api/v1/auth/register", `{"email":"a@example.test","password":"password123"}`, &authServiceStub{registerResult: appauth.Registration{User: domain.User{ID: "11111111-1111-4111-8111-111111111111", Email: "a@example.test", CreatedAt: time.Now().UTC()}, Session: session}}, "application/json"},
		{"/api/v1/auth/register", `{}`, &authServiceStub{registerErr: appauth.ErrInvalidEmail}, "application/json"},
		{"/api/v1/auth/register", `{}`, &authServiceStub{registerErr: appauth.ErrEmailAlreadyExists}, "application/json"},
		{"/api/v1/auth/login", `{"email":"a@example.test","password":"password123"}`, &authServiceStub{loginResult: session}, "application/json"},
		{"/api/v1/auth/login", `{}`, &authServiceStub{loginErr: appauth.ErrInvalidCredentials}, "application/json"},
		{"/api/v1/auth/login", `{}`, &authServiceStub{loginErr: appauth.ErrUserBlocked}, "application/json"},
		{"/api/v1/auth/login", `{}`, &authServiceStub{loginErr: errors.New("SECRET")}, "application/json"},
		{"/api/v1/auth/login", `{}`, &authServiceStub{}, ""},
		{"/api/v1/auth/login", `{}`, &authServiceStub{}, "text/plain"},
		{"/api/v1/auth/login", `{}` + strings.Repeat(" ", 17000), &authServiceStub{}, "application/json"},
		{"/api/v1/auth/refresh", `{"refresh_token":"refresh"}`, &authServiceStub{refreshResult: session}, "application/json"},
		{"/api/v1/auth/refresh", `{"refresh_token":"refresh"}`, &authServiceStub{refreshErr: appauth.ErrInvalidRefreshToken}, "application/json"},
		{"/api/v1/auth/refresh", `{"refresh_token":"refresh"}`, &authServiceStub{refreshErr: errors.New("SECRET")}, "application/json"},
		{"/api/v1/auth/logout", `{"refresh_token":"refresh"}`, &authServiceStub{}, "application/json"},
		{"/api/v1/auth/logout", `{"refresh_token":"refresh"}`, &authServiceStub{logoutErr: appauth.ErrInvalidRefreshToken}, "application/json"},
		{"/api/v1/auth/logout", `{"refresh_token":"refresh"}`, &authServiceStub{logoutErr: errors.New("SECRET")}, "application/json"},
	} {
		mux := http.NewServeMux()
		RegisterRoutes(mux, NewHandler(test.service, nil), middleware.RequireUser(test.service))
		request := httptest.NewRequest("POST", test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", test.contentType)
		request.Header.Set("Authorization", "Bearer valid-access")
		response := httptest.NewRecorder()
		middleware.RequestID(mux).ServeHTTP(response, request)
		spec.CheckResponse(t, "POST", test.path, response)
		if strings.Contains(response.Body.String(), "SECRET") {
			t.Fatal("internal auth error leaked")
		}
	}
}
