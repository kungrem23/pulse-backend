package auth

import (
	"net/http"

	appauth "github.com/example/pulse-backend/internal/app/auth"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appauth.Service
}

func NewHandler(service appauth.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("register user").ServeHTTP(w, r)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("login").ServeHTTP(w, r)
}

func (h *Handler) LoginAdmin(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("admin login").ServeHTTP(w, r)
}
