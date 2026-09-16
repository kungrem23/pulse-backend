package matching

import (
	"net/http"

	appmatching "github.com/example/pulse-backend/internal/app/matching"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appmatching.Service
}

func NewHandler(service appmatching.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Swipe(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("like or dislike").ServeHTTP(w, r)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("list matches").ServeHTTP(w, r)
}
