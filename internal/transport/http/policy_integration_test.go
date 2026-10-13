package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type policyAuth struct{ appauth.Service }

func (policyAuth) Login(context.Context, appauth.LoginCommand) (appauth.Session, error) {
	return appauth.Session{}, appauth.ErrInvalidCredentials
}
func (policyAuth) Authenticate(context.Context, string) (string, error) {
	return "", appauth.ErrInvalidAccessToken
}

func TestRouterHTTPPolicy(t *testing.T) {
	limiter, err := middleware.NewRateLimiter(middleware.RateLimit{Requests: 1, Window: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://frontend.example"
	router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: policyAuth{}, AuthRateLimiter: limiter, CORSOrigins: []string{origin}})
	spec := contracttest.Load(t)
	send := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(`{"email":"a@example.test","password":"password123"}`))
		request.RemoteAddr = "203.0.113.10:54321"
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get(httpx.RequestIDHeader) == "" {
			t.Fatal("router did not attach request ID")
		}
		if response.Code >= 400 {
			var body httpx.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.RequestID != response.Header().Get(httpx.RequestIDHeader) {
				t.Fatalf("error request ID mismatch: %s", response.Body)
			}
		}
		return response
	}
	headers := map[string]string{"Origin": origin, "Content-Type": "application/json", "X-Request-ID": "client-123"}
	first := send("POST", "/api/v1/auth/login", headers)
	if first.Code != 401 || first.Header().Get(httpx.RequestIDHeader) != "client-123" {
		t.Fatalf("unexpected first login: %d", first.Code)
	}
	spec.CheckResponse(t, "POST", "/api/v1/auth/login", first)
	limited := send("POST", "/api/v1/auth/login", headers)
	if limited.Code != 429 || limited.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("rate limit/CORS not wired: %d %v", limited.Code, limited.Header())
	}
	if retry, err := strconv.Atoi(limited.Header().Get("Retry-After")); err != nil || retry < 1 {
		t.Fatal("429 missing Retry-After")
	}
	spec.CheckResponse(t, "POST", "/api/v1/auth/login", limited)
	preflight := send("OPTIONS", "/api/v1/auth/login", map[string]string{"Origin": origin, "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "Authorization, Content-Type"})
	if preflight.Code != 204 || preflight.Body.Len() != 0 {
		t.Fatalf("preflight must bypass auth and limiter: %d", preflight.Code)
	}
	foreign := send("OPTIONS", "/api/v1/auth/login", map[string]string{"Origin": "https://foreign.example", "Access-Control-Request-Method": "POST"})
	if foreign.Code != 403 || foreign.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign preflight was accepted")
	}
	live := send("GET", "/health/live", nil)
	if live.Code != 200 {
		t.Fatal("auth limiter affected health")
	}
	spec.CheckResponse(t, "GET", "/health/live", live)
	unknown := send("GET", "/unknown", nil)
	if unknown.Code != 404 || !strings.Contains(unknown.Body.String(), `"code":"not_found"`) {
		t.Fatal("router 404 is not JSON")
	}
	wrongMethod := send("POST", "/health/live", nil)
	if wrongMethod.Code != 405 || wrongMethod.Header().Get("Allow") == "" || !strings.Contains(wrongMethod.Body.String(), `"method_not_allowed"`) {
		t.Fatal("router 405 lost JSON or Allow")
	}
	// One route's exhausted budget must not consume another auth route's tokens.
	other := send("POST", "/api/v1/auth/refresh", headers)
	if other.Code == 429 {
		t.Fatal("route budgets are not independent")
	}
}
