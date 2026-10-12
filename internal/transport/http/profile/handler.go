package profile

import (
	"net/http"

	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appprofile.Service
}

func NewHandler(service appprofile.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("get own profile").ServeHTTP(w, r)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("create or update profile").ServeHTTP(w, r)
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("update preferences").ServeHTTP(w, r)
}

func (h *Handler) SetSleep(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("toggle sleep mode").ServeHTTP(w, r)
}
