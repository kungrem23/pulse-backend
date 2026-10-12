package health

import (
	"context"
	"net/http"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

const readinessTimeout = 3 * time.Second

type Pinger interface {
	Ping(context.Context) error
}

type Handler struct {
	checks []Pinger
}

func NewHandler(checks ...Pinger) *Handler {
	return &Handler{checks: checks}
}

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()
	for _, check := range h.checks {
		if check == nil || check.Ping(ctx) != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "dependency is unavailable")
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
