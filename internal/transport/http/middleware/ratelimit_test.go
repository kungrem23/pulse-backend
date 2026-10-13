package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

const loginRoute = "POST /api/v1/auth/login"

func newTestLimiter(t *testing.T, config RateLimit) (*RateLimiter, *fakeClock) {
	t.Helper()
	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	limiter.now = clock.Now
	return limiter, clock
}

func limitedHandler(limiter *RateLimiter) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return limiter.LimitRoutes(ok, loginRoute, "POST /api/v1/auth/register")
}

func call(handler http.Handler, method, path, ip string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = ip + ":40000"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRateLimitConfigValidation(t *testing.T) {
	for _, config := range []RateLimit{
		{Requests: 0, Window: time.Minute},
		{Requests: 5, Window: 500 * time.Millisecond},
		{Requests: 5, Window: time.Minute, MaxKeys: -1},
	} {
		if _, err := NewRateLimiter(config); err == nil {
			t.Fatalf("%+v must be rejected", config)
		}
	}
}

func TestRateLimitBurstThenRetryAfter(t *testing.T) {
	limiter, clock := newTestLimiter(t, RateLimit{Requests: 3, Window: time.Minute})
	handler := RequestID(limitedHandler(limiter))
	spec := contracttest.Load(t)

	for i := 0; i < 3; i++ {
		if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1"); response.Code != http.StatusNoContent {
			t.Fatalf("request %d within the burst: got %d", i+1, response.Code)
		}
	}
	limited := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") != "20" {
		t.Fatalf("expected 429 with Retry-After 20 (one token per 20s), got %d %q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if body := decodeEnvelope(t, limited); body.Error.Code != "too_many_requests" || body.RequestID == "" {
		t.Fatalf("unexpected envelope %+v", body)
	}
	limited = call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1")
	spec.CheckResponse(t, http.MethodPost, "/api/v1/auth/login", limited)

	clock.Advance(19 * time.Second)
	if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1"); response.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After 1 just before the refill, got %d %q", response.Code, response.Header().Get("Retry-After"))
	}
	clock.Advance(time.Second)
	if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1"); response.Code != http.StatusNoContent {
		t.Fatalf("a token must be refilled after 20s, got %d", response.Code)
	}
}

func TestRateLimitIsPerClientAndRoute(t *testing.T) {
	limiter, _ := newTestLimiter(t, RateLimit{Requests: 1, Window: time.Minute})
	handler := limitedHandler(limiter)

	call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1")
	if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", response.Code)
	}
	for name, response := range map[string]*httptest.ResponseRecorder{
		"other client":         call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.2"),
		"other limited route":  call(handler, http.MethodPost, "/api/v1/auth/register", "203.0.113.1"),
		"route not limited":    call(handler, http.MethodPost, "/api/v1/swipes", "203.0.113.1"),
		"other method":         call(handler, http.MethodGet, "/api/v1/auth/login", "203.0.113.1"),
		"ipv6 client":          call(handler, http.MethodPost, "/api/v1/auth/login", "[2001:db8::1]"),
		"forwarded header lie": forwarded(handler),
	} {
		want := http.StatusNoContent
		if name == "forwarded header lie" {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("%s: expected %d, got %d", name, want, response.Code)
		}
	}
}

// forwarded shows that X-Forwarded-For does not give a client a fresh budget.
func forwarded(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "203.0.113.1:40000"
	request.Header.Set("X-Forwarded-For", "198.51.100.9")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRateLimitBoundsMemory(t *testing.T) {
	limiter, clock := newTestLimiter(t, RateLimit{Requests: 1, Window: time.Minute, MaxKeys: 2})
	handler := limitedHandler(limiter)

	call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1")
	call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.2")
	if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.3"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("a new client over MaxKeys must be limited, got %d", response.Code)
	}
	clock.Advance(time.Minute)
	if response := call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.3"); response.Code != http.StatusNoContent {
		t.Fatalf("idle buckets must be swept after a window, got %d", response.Code)
	}
	if len(limiter.buckets) != 1 {
		t.Fatalf("expected only the new bucket after the sweep, got %d", len(limiter.buckets))
	}
}

func TestRateLimitConcurrentRequests(t *testing.T) {
	limiter, _ := newTestLimiter(t, RateLimit{Requests: 10, Window: time.Hour})
	handler := limitedHandler(limiter)
	var mu sync.Mutex
	allowed := 0
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if call(handler, http.MethodPost, "/api/v1/auth/login", "203.0.113.1").Code == http.StatusNoContent {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("expected exactly the burst of 10 to pass, got %d", allowed)
	}
}
