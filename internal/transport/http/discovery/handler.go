package discovery

import (
	"errors"
	"net/http"

	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type Handler struct {
	service appdiscovery.Service
}

func NewHandler(service appdiscovery.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	page, err := httpx.ParsePageRequest(r)
	if page.Limit == 0 {
		page.Limit = pagination.DefaultLimit
	}
	if page.Limit > pagination.MaxLimit {
		err = domain.ErrInvalidLimit
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid feed pagination parameters")
		return
	}
	if h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	result, err := h.service.Feed(r.Context(), userID, page)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidCursor), errors.Is(err, domain.ErrInvalidLimit):
			httpx.Error(w, http.StatusBadRequest, "invalid feed pagination parameters")
		case errors.Is(err, domain.ErrInvalidUserID), errors.Is(err, domain.ErrNotFound):
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
		case errors.Is(err, domain.ErrProfileIncomplete), errors.Is(err, domain.ErrNoPhotos),
			errors.Is(err, domain.ErrSleeping), errors.Is(err, domain.ErrBlocked):
			httpx.Error(w, http.StatusForbidden, "profile is not eligible for discovery")
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newFeedResponse(result))
}
