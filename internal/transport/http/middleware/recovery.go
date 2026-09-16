package middleware

import (
	"log/slog"
	"net/http"

	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("panic recovered", "value", value)
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
