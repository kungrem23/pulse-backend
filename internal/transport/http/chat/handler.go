package chat

import (
	"net/http"

	appchat "github.com/example/pulse-backend/internal/app/chat"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appchat.Service
}

func NewHandler(service appchat.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("message history").ServeHTTP(w, r)
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("send message").ServeHTTP(w, r)
}
