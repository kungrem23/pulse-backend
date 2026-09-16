package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	requests atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Count(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.requests.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "# HELP pulse_up Whether the API process is running.\n# TYPE pulse_up gauge\npulse_up 1\n# HELP pulse_http_requests_total Total HTTP requests.\n# TYPE pulse_http_requests_total counter\npulse_http_requests_total %d\n", m.requests.Load())
}
