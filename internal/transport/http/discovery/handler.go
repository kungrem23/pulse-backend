package discovery

import (
	"net/http"

	appdiscovery "github.com/example/pulse-backend/internal/app/discovery"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appdiscovery.Service
}

func NewHandler(service appdiscovery.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("recommendation feed").ServeHTTP(w, r)
}
