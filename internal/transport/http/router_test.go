package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

func TestLiveness(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(Dependencies{Logger: logger})
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

// authStub authenticates "valid" and rejects "blocked"; other methods are unused.
type authStub struct{ appauth.Service }

func (authStub) Authenticate(_ context.Context, token string) (string, error) {
	switch token {
	case "valid":
		return "6130ce38-a239-447c-b65a-a2019b370962", nil
	case "blocked":
		return "", appauth.ErrUserBlocked
	}
	return "", appauth.ErrInvalidAccessToken
}

const sampleID = "6130ce38-a239-447c-b65a-a2019b370962"

// Every operation with x-required-principal: user, except logout which is implemented.
var userRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/profile"},
	{http.MethodPut, "/api/v1/profile"},
	{http.MethodPut, "/api/v1/preferences"},
	{http.MethodPatch, "/api/v1/profile/sleep"},
	{http.MethodPost, "/api/v1/photos"},
	{http.MethodDelete, "/api/v1/photos/" + sampleID},
	{http.MethodGet, "/api/v1/feed"},
	{http.MethodPost, "/api/v1/swipes"},
	{http.MethodGet, "/api/v1/matches"},
	{http.MethodDelete, "/api/v1/matches/" + sampleID},
	{http.MethodGet, "/api/v1/conversations"},
	{http.MethodGet, "/api/v1/conversations/" + sampleID + "/messages"},
	{http.MethodPost, "/api/v1/conversations/" + sampleID + "/messages"},
	{http.MethodPost, "/api/v1/reports"},
}

func TestUserRoutesRequireUnblockedUser(t *testing.T) {
	router := NewRouter(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:   authStub{},
	})
	for _, route := range userRoutes {
		for _, test := range []struct {
			name       string
			header     string
			wantStatus int
			wantCode   string
		}{
			{"no token", "", http.StatusUnauthorized, "unauthorized"},
			{"invalid token", "Bearer forged", http.StatusUnauthorized, "unauthorized"},
			{"blocked user", "Bearer blocked", http.StatusForbidden, "forbidden"},
			{"valid user", "Bearer valid", http.StatusNotImplemented, "not_implemented"},
		} {
			t.Run(route.method+" "+route.path+"/"+test.name, func(t *testing.T) {
				request := httptest.NewRequest(route.method, route.path, nil)
				if test.header != "" {
					request.Header.Set("Authorization", test.header)
				}
				res := httptest.NewRecorder()
				router.ServeHTTP(res, request)
				var body httpx.ErrorResponse
				if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if res.Code != test.wantStatus || body.Error.Code != test.wantCode {
					t.Fatalf("expected %d %s, got %d %s", test.wantStatus, test.wantCode, res.Code, body.Error.Code)
				}
			})
		}
	}
}

func TestUserRoutesFailClosedWithoutAuthService(t *testing.T) {
	router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	request.Header.Set("Authorization", "Bearer valid")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, request)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.Code)
	}
}

func TestAdminPlaceholdersStayNotImplemented(t *testing.T) {
	router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/reports"},
		{http.MethodPatch, "/api/v1/admin/reports/" + sampleID},
		{http.MethodGet, "/api/v1/admin/users/" + sampleID},
		{http.MethodPost, "/api/v1/admin/users/" + sampleID + "/block"},
		{http.MethodPost, "/api/v1/admin/users/" + sampleID + "/messages"},
		{http.MethodPost, "/api/v1/admin/users/" + sampleID + "/notifications"},
	} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(route.method, route.path, nil))
		if res.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s: expected 501, got %d", route.method, route.path, res.Code)
		}
	}
}
