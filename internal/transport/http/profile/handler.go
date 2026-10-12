package profile

import (
	"errors"
	"net/http"

	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type Handler struct {
	service appprofile.Service
}

func NewHandler(service appprofile.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok || h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	view, err := h.service.Get(r.Context(), userID)
	if err != nil {
		if errors.Is(err, appprofile.ErrProfileNotFound) {
			httpx.Error(w, http.StatusNotFound, "profile not found")
		} else {
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newViewResponse(view))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok || h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var request updateRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile, err := h.service.Update(r.Context(), appprofile.UpdateCommand{
		UserID: userID,
		Name:   request.Name,
		Bio:    request.Bio,
		Age:    request.Age,
		Sex:    request.Sex,
		City:   request.City,
	})
	if err != nil {
		var validation *appprofile.ValidationError
		switch {
		case errors.As(err, &validation):
			httpx.ValidationFailed(w, validation.Field, validation.Message)
		case errors.Is(err, appprofile.ErrUserNotFound):
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newProfileResponse(profile))
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("update preferences").ServeHTTP(w, r)
}

func (h *Handler) SetSleep(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("toggle sleep mode").ServeHTTP(w, r)
}
