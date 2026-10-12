package health

import (
	"context"
	"net/http"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

type Pinger interface {
	Ping(context.Context) error
}

type Handler struct {
	database Pinger
}

func NewHandler(database Pinger) *Handler {
	return &Handler{database: database}
}

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.database == nil || h.database.Ping(r.Context()) != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
