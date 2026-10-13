package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

// The order proposed for router.go in AI/AI/WORKLOG.md:
// RequestID -> Recover -> metrics -> Logging -> CORS -> rate limit -> JSONErrors -> mux.
func TestRecommendedStack(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter, err := NewRateLimiter(RateLimit{Requests: 1, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"access_token": "x"})
	})
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	metrics := NewMetrics()
	handler := RequestID(Recover(logger, metrics.Count(Logging(logger,
		CORS([]string{frontend})(limiter.LimitRoutes(JSONErrors(mux), "POST /api/v1/auth/login"))))))

	serve := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.RemoteAddr = "203.0.113.1:1234"
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	cors := map[string]string{"Origin": frontend}

	if response := serve(http.MethodPost, "/api/v1/auth/login", cors); response.Code != http.StatusOK ||
		response.Header().Get("Access-Control-Allow-Origin") != frontend || response.Header().Get(httpx.RequestIDHeader) == "" {
		t.Fatalf("unexpected first login %d %v", response.Code, response.Header())
	}
	limited := serve(http.MethodPost, "/api/v1/auth/login", cors)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Access-Control-Allow-Origin") != frontend {
		t.Fatalf("429 must keep CORS headers so the browser can read Retry-After, got %d %v", limited.Code, limited.Header())
	}

	preflight := serve(http.MethodOptions, "/api/v1/auth/login", map[string]string{"Origin": frontend, "Access-Control-Request-Method": "POST"})
	if preflight.Code != http.StatusNoContent {
		t.Fatalf("preflight must not be rate limited or routed, got %d", preflight.Code)
	}

	panicked := serve(http.MethodGet, "/panic", nil)
	if body := decodeEnvelope(t, panicked); panicked.Code != http.StatusInternalServerError || body.RequestID == "" ||
		body.RequestID != panicked.Header().Get(httpx.RequestIDHeader) {
		t.Fatalf("panic must give a JSON 500 with request_id, got %d %+v", panicked.Code, body)
	}

	unknown := serve(http.MethodGet, "/nope", nil)
	if body := decodeEnvelope(t, unknown); unknown.Code != http.StatusNotFound || body.RequestID == "" {
		t.Fatalf("unknown route must give a JSON 404 with request_id, got %d %+v", unknown.Code, body)
	}
}
