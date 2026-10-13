package middleware

import (
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

// DefaultMaxRateLimitKeys bounds the memory of one RateLimiter.
const DefaultMaxRateLimitKeys = 100_000

// RateLimit allows Requests per Window for each client and route, with bursts
// of up to Requests. The values are configuration: the owner has not fixed
// production thresholds.
type RateLimit struct {
	Requests int
	Window   time.Duration
	// MaxKeys bounds tracked clients; 0 means DefaultMaxRateLimitKeys.
	MaxKeys int
}

func (c RateLimit) Validate() error {
	switch {
	case c.Requests < 1:
		return errors.New("rate limit requests must be positive")
	case c.Window < time.Second:
		return errors.New("rate limit window must be at least 1s")
	case c.MaxKeys < 0:
		return errors.New("rate limit max keys must not be negative")
	}
	return nil
}

// RateLimiter is an in-memory token bucket per client IP and route. It is
// per process: several API instances each apply their own limit.
type RateLimiter struct {
	mu        sync.Mutex
	burst     float64
	perSecond float64
	window    time.Duration
	maxKeys   int
	buckets   map[string]*bucket
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	tokens  float64
	updated time.Time
}

func NewRateLimiter(config RateLimit) (*RateLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	maxKeys := config.MaxKeys
	if maxKeys == 0 {
		maxKeys = DefaultMaxRateLimitKeys
	}
	return &RateLimiter{
		burst:     float64(config.Requests),
		perSecond: float64(config.Requests) / config.Window.Seconds(),
		window:    config.Window,
		maxKeys:   maxKeys,
		buckets:   map[string]*bucket{},
		now:       time.Now,
	}, nil
}

// LimitRoutes applies the limiter to the listed "METHOD /exact/path" routes
// and passes every other request through. Each route has its own budget, so
// frequent refreshes do not use up the login attempts.
func (l *RateLimiter) LimitRoutes(next http.Handler, routes ...string) http.Handler {
	limited := make(map[string]bool, len(routes))
	for _, route := range routes {
		limited[route] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Method + " " + r.URL.Path
		if !limited[route] {
			next.ServeHTTP(w, r)
			return
		}
		if allowed, retryAfter := l.allow(route + " " + clientIP(r)); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.Error(w, http.StatusTooManyRequests, "too many requests, retry later")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow takes a token and otherwise returns whole seconds until the next one.
func (l *RateLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)
	current, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			// Fail closed instead of growing without bound under a flood of
			// new clients; known clients keep their buckets.
			return false, int(math.Ceil(l.window.Seconds()))
		}
		current = &bucket{tokens: l.burst, updated: now}
		l.buckets[key] = current
	}
	elapsed := now.Sub(current.updated).Seconds()
	if elapsed > 0 {
		current.tokens = math.Min(l.burst, current.tokens+elapsed*l.perSecond)
		current.updated = now
	}
	if current.tokens >= 1 {
		current.tokens--
		return true, 0
	}
	wait := math.Ceil((1 - current.tokens) / l.perSecond)
	return false, int(math.Max(1, wait))
}

// sweep drops buckets that have been idle for a full window: they are full
// again and behave exactly like new ones.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window && len(l.buckets) < l.maxKeys {
		return
	}
	l.lastSweep = now
	for key, current := range l.buckets {
		if now.Sub(current.updated) >= l.window {
			delete(l.buckets, key)
		}
	}
}

// clientIP uses the connection address. X-Forwarded-For is not trusted: a
// client could set it to bypass the limit. Behind a reverse proxy all clients
// share the proxy address until a trusted-proxy setting is added.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
