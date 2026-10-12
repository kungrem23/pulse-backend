package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/app/adminauth"
	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type authServiceStub struct {
	registerResult  appauth.Registration
	registerErr     error
	loginResult     appauth.Session
	loginErr        error
	registerCommand appauth.RegisterCommand
	loginCommand    appauth.LoginCommand
	registerCalls   int
	loginCalls      int
	refreshResult   appauth.Session
	refreshErr      error
	refreshCommand  appauth.RefreshCommand
	logoutErr       error
	logoutCommand   appauth.LogoutCommand
	logoutCalls     int
}

func (s *authServiceStub) Refresh(_ context.Context, command appauth.RefreshCommand) (appauth.Session, error) {
	s.refreshCommand = command
	return s.refreshResult, s.refreshErr
}

func (s *authServiceStub) Logout(_ context.Context, command appauth.LogoutCommand) error {
	s.logoutCalls++
	s.logoutCommand = command
	return s.logoutErr
}

func (s *authServiceStub) Register(_ context.Context, command appauth.RegisterCommand) (appauth.Registration, error) {
	s.registerCalls++
	s.registerCommand = command
	return s.registerResult, s.registerErr
}

func (s *authServiceStub) Login(_ context.Context, command appauth.LoginCommand) (appauth.Session, error) {
	s.loginCalls++
	s.loginCommand = command
	return s.loginResult, s.loginErr
}

type adminAuthServiceStub struct {
	result  adminauth.Session
	err     error
	command adminauth.LoginCommand
	calls   int
}

func (s *adminAuthServiceStub) Login(_ context.Context, command adminauth.LoginCommand) (adminauth.Session, error) {
	s.calls++
	s.command = command
	return s.result, s.err
}

func TestRegisterCallsServiceAndReturnsRegistration(t *testing.T) {
	accessExpiresAt := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	refreshExpiresAt := accessExpiresAt.Add(24 * time.Hour)
	service := &authServiceStub{
		registerResult: appauth.Registration{
			User: domain.User{
				ID:        "user-1",
				Email:     "user@example.com",
				Age:       18,
				CreatedAt: accessExpiresAt,
			},
			Session: appauth.Session{
				AccessToken:      "access-token",
				RefreshToken:     "refresh-token",
				AccessExpiresAt:  accessExpiresAt,
				RefreshExpiresAt: refreshExpiresAt,
			},
		},
	}
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(
		`{"email":"user@example.com","password":"password123","age":18}`,
	))
	response := httptest.NewRecorder()

	handler.Register(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if service.registerCalls != 1 {
		t.Fatalf("expected one service call, got %d", service.registerCalls)
	}
	if service.registerCommand != (appauth.RegisterCommand{Email: "user@example.com", Password: "password123", Age: 18}) {
		t.Fatalf("unexpected command: %+v", service.registerCommand)
	}

	var body registerResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.User.ID != "user-1" || body.Session.AccessToken != "access-token" || body.Session.TokenType != "Bearer" {
		t.Fatalf("unexpected response: %+v", body)
	}
	if !body.Session.AccessExpiresAt.Equal(accessExpiresAt) || !body.Session.RefreshExpiresAt.Equal(refreshExpiresAt) {
		t.Fatalf("unexpected session expiration times: %+v", body.Session)
	}
}

func TestLoginCallsUserService(t *testing.T) {
	service := &authServiceStub{
		loginResult: appauth.Session{AccessToken: "user-access", RefreshToken: "user-refresh"},
	}
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(
		`{"email":"user@example.com","password":"secret"}`,
	))
	response := httptest.NewRecorder()

	handler.Login(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if service.loginCommand != (appauth.LoginCommand{Email: "user@example.com", Password: "secret"}) {
		t.Fatalf("unexpected command: %+v", service.loginCommand)
	}
}

func TestLoginAdminCallsAdminService(t *testing.T) {
	adminService := &adminAuthServiceStub{
		result: adminauth.Session{AccessToken: "admin-access", RefreshToken: "admin-refresh"},
	}
	handler := NewHandler(nil, adminService)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", bytes.NewBufferString(
		`{"username":"moderator","password":"secret"}`,
	))
	response := httptest.NewRecorder()

	handler.LoginAdmin(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if adminService.command != (adminauth.LoginCommand{Username: "moderator", Password: "secret"}) {
		t.Fatalf("unexpected command: %+v", adminService.command)
	}
}

func TestRegisterMapsApplicationErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid email", err: appauth.ErrInvalidEmail, wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid age", err: appauth.ErrInvalidAge, wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid password", err: appauth.ErrInvalidPassword, wantStatus: http.StatusUnprocessableEntity},
		{name: "email exists", err: appauth.ErrEmailAlreadyExists, wantStatus: http.StatusConflict},
		{name: "unexpected", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &authServiceStub{registerErr: test.err}
			handler := NewHandler(service, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(
				`{"email":"user@example.com","password":"password123","age":18}`,
			))
			response := httptest.NewRecorder()

			handler.Register(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
		})
	}
}

func TestLoginMapsApplicationErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid credentials", err: appauth.ErrInvalidCredentials, wantStatus: http.StatusUnauthorized},
		{name: "blocked", err: appauth.ErrUserBlocked, wantStatus: http.StatusForbidden},
		{name: "unexpected", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &authServiceStub{loginErr: test.err}
			handler := NewHandler(service, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(
				`{"email":"user@example.com","password":"secret"}`,
			))
			response := httptest.NewRecorder()

			handler.Login(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
		})
	}
}

func TestLoginAdminMapsInvalidCredentials(t *testing.T) {
	handler := NewHandler(nil, &adminAuthServiceStub{err: adminauth.ErrInvalidCredentials})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", bytes.NewBufferString(
		`{"username":"moderator","password":"wrong"}`,
	))
	response := httptest.NewRecorder()

	handler.LoginAdmin(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d: %s", http.StatusUnauthorized, response.Code, response.Body.String())
	}
}

func TestInvalidJSONDoesNotCallService(t *testing.T) {
	service := &authServiceStub{}
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(
		`{"email":"user@example.com","password":"secret","extra":true}`,
	))
	response := httptest.NewRecorder()

	handler.Login(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if service.loginCalls != 0 {
		t.Fatalf("expected no service calls, got %d", service.loginCalls)
	}
}

func TestRefreshMapsResults(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "rotated", body: `{"refresh_token":"old"}`, wantStatus: http.StatusOK},
		{name: "empty token", body: `{"refresh_token":""}`, wantStatus: http.StatusBadRequest},
		{name: "invalid token", body: `{"refresh_token":"old"}`, err: appauth.ErrInvalidRefreshToken, wantStatus: http.StatusUnauthorized},
		{name: "blocked user", body: `{"refresh_token":"old"}`, err: appauth.ErrUserBlocked, wantStatus: http.StatusUnauthorized},
		{name: "unexpected", body: `{"refresh_token":"old"}`, err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &authServiceStub{refreshErr: test.err, refreshResult: appauth.Session{AccessToken: "new-access"}}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(test.body))
			response := httptest.NewRecorder()

			NewHandler(service, nil).Refresh(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			if test.wantStatus == http.StatusOK && service.refreshCommand.RefreshToken != "old" {
				t.Fatalf("unexpected command: %+v", service.refreshCommand)
			}
		})
	}
}

func (s *authServiceStub) Authenticate(_ context.Context, token string) (string, error) {
	if token == "valid-access" {
		return "user-from-token", nil
	}
	return "", appauth.ErrInvalidAccessToken
}

func TestLogoutTakesUserFromVerifiedAccessToken(t *testing.T) {
	service := &authServiceStub{}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service, nil), middleware.RequireUser(service))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{"refresh_token":"refresh"}`))
	request.Header.Set("Authorization", "Bearer valid-access")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("expected empty 204, got %d: %s", response.Code, response.Body.String())
	}
	if service.logoutCommand != (appauth.LogoutCommand{UserID: "user-from-token", RefreshToken: "refresh"}) {
		t.Fatalf("unexpected command: %+v", service.logoutCommand)
	}
}

func TestLogoutRejectsMissingOrWrongToken(t *testing.T) {
	for name, header := range map[string]string{
		"missing":               "",
		"invalid access token":  "Bearer forged",
		"invalid refresh token": "Bearer valid-access",
	} {
		t.Run(name, func(t *testing.T) {
			service := &authServiceStub{}
			if name == "invalid refresh token" {
				service.logoutErr = appauth.ErrInvalidRefreshToken
			}
			mux := http.NewServeMux()
			RegisterRoutes(mux, NewHandler(service, nil), middleware.RequireUser(service))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{"refresh_token":"refresh"}`))
			if header != "" {
				request.Header.Set("Authorization", header)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
			}
			if name != "invalid refresh token" && service.logoutCalls != 0 {
				t.Fatal("unauthenticated request must not reach the service")
			}
		})
	}
}
