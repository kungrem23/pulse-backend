package media

import (
	"net/http"

	appmedia "github.com/example/pulse-backend/internal/app/media"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appmedia.Service
}

func NewHandler(service appmedia.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("upload photo").ServeHTTP(w, r)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("delete photo").ServeHTTP(w, r)
}
