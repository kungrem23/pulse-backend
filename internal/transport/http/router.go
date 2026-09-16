package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(logger *slog.Logger, db *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	var requests atomic.Uint64
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := db.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP pulse_up Whether the API process is running.\n# TYPE pulse_up gauge\npulse_up 1\n# HELP pulse_http_requests_total Total HTTP requests.\n# TYPE pulse_http_requests_total counter\npulse_http_requests_total " + formatUint(requests.Load()) + "\n"))
	})

	// Public API.
	mux.HandleFunc("POST /api/v1/auth/register", notImplemented("register user"))
	mux.HandleFunc("POST /api/v1/auth/login", notImplemented("login"))
	mux.HandleFunc("POST /api/v1/admin/auth/login", notImplemented("admin login"))

	// User API. Add auth middleware when token service is implemented.
	mux.HandleFunc("GET /api/v1/profile", notImplemented("get own profile"))
	mux.HandleFunc("PUT /api/v1/profile", notImplemented("create or update profile"))
	mux.HandleFunc("PUT /api/v1/preferences", notImplemented("update preferences"))
	mux.HandleFunc("POST /api/v1/photos", notImplemented("upload photo"))
	mux.HandleFunc("DELETE /api/v1/photos/{photoID}", notImplemented("delete photo"))
	mux.HandleFunc("GET /api/v1/feed", notImplemented("recommendation feed"))
	mux.HandleFunc("POST /api/v1/swipes", notImplemented("like or dislike"))
	mux.HandleFunc("GET /api/v1/matches", notImplemented("list matches"))
	mux.HandleFunc("GET /api/v1/matches/{matchID}/messages", notImplemented("message history"))
	mux.HandleFunc("POST /api/v1/matches/{matchID}/messages", notImplemented("send message"))
	mux.HandleFunc("POST /api/v1/reports", notImplemented("report user"))
	mux.HandleFunc("PATCH /api/v1/profile/sleep", notImplemented("toggle sleep mode"))

	// Admin API. Protect with admin role middleware.
	mux.HandleFunc("GET /api/v1/admin/reports", notImplemented("list reports"))
	mux.HandleFunc("PATCH /api/v1/admin/reports/{reportID}", notImplemented("process report"))
	mux.HandleFunc("GET /api/v1/admin/users/{userID}", notImplemented("inspect user"))
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/block", notImplemented("block user"))
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/messages", notImplemented("message user"))
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/notifications", notImplemented("notify user"))

	return recoverer(logger, countRequests(&requests, requestLogger(logger, mux)))
}

func formatUint(value uint64) string {
	return fmt.Sprintf("%d", value)
}

func countRequests(counter *atomic.Uint64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		next.ServeHTTP(w, r)
	})
}

func notImplemented(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error":     "not implemented",
			"operation": operation,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("panic recovered", "value", value)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
