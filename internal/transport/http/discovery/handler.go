package discovery

import (
	"net/http"

	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
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
