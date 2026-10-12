package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestContractPlaceholderRoutesAreRegistered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(Dependencies{Logger: logger})
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/auth/refresh"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/conversations"},
		{http.MethodGet, "/api/v1/conversations/6130ce38-a239-447c-b65a-a2019b370962/messages"},
		{http.MethodPost, "/api/v1/conversations/6130ce38-a239-447c-b65a-a2019b370962/messages"},
		{http.MethodDelete, "/api/v1/matches/6130ce38-a239-447c-b65a-a2019b370962"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			router.ServeHTTP(res, httptest.NewRequest(route.method, route.path, nil))
			var body httpx.ErrorResponse
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if res.Code != http.StatusNotImplemented || body.Error.Code != "not_implemented" {
				t.Fatalf("expected a registered 501 placeholder, got %d", res.Code)
			}
		})
	}
}
