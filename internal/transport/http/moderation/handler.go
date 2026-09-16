package moderation

import (
	"net/http"

	appmoderation "github.com/example/pulse-backend/internal/app/moderation"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service appmoderation.Service
}

func NewHandler(service appmoderation.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateReport(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("report user").ServeHTTP(w, r)
}

func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("list reports").ServeHTTP(w, r)
}

func (h *Handler) ProcessReport(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("process report").ServeHTTP(w, r)
}

func (h *Handler) InspectUser(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("inspect user").ServeHTTP(w, r)
}

func (h *Handler) BlockUser(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("block user").ServeHTTP(w, r)
}

func (h *Handler) MessageUser(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("message user").ServeHTTP(w, r)
}

func (h *Handler) NotifyUser(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("notify user").ServeHTTP(w, r)
}
