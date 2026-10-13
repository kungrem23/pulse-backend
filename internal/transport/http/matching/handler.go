package matching

import (
	"errors"
	"net/http"

	appmatching "github.com/kungrem23/pulse-backend/internal/app/matching"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type Handler struct {
	service appmatching.Service
}

func NewHandler(service appmatching.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Swipe(w http.ResponseWriter, r *http.Request) {
	// Until the swipe repository is wired in cmd/api the route stays unavailable.
	if h.service == nil {
		httpx.NotImplemented("like or dislike").ServeHTTP(w, r)
		return
	}
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var request swipeRequest
	if err := httpx.DecodeJSON(r, &request); err != nil || request.IsLike == nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	match, err := h.service.Swipe(r.Context(), appmatching.SwipeCommand{
		FromUserID: userID,
		ToUserID:   request.ToUserID,
		IsLike:     *request.IsLike,
	})
	if err != nil {
		writeSwipeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newSwipeResponse(match))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	// Until the match repository is wired in cmd/api the route stays unavailable.
	if h.service == nil {
		httpx.NotImplemented("list matches").ServeHTTP(w, r)
		return
	}
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	request, err := httpx.ParsePageRequest(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid cursor or limit")
		return
	}

	page, err := h.service.List(r.Context(), userID, request)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, newMatchPageResponse(page))
	case errors.Is(err, domain.ErrInvalidCursor), errors.Is(err, domain.ErrInvalidLimit):
		httpx.Error(w, http.StatusBadRequest, "invalid cursor or limit")
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// Close is post-MVP: closing a match comes with chat.
func (h *Handler) Close(w http.ResponseWriter, r *http.Request) {
	httpx.NotImplemented("close match permanently").ServeHTTP(w, r)
}

// eligibilityErrors are the caller's own state; their messages are safe to show.
var eligibilityErrors = []error{
	domain.ErrBlocked, domain.ErrProfileIncomplete, domain.ErrNoPhotos, domain.ErrSleeping,
}

func writeSwipeError(w http.ResponseWriter, err error) {
	for _, eligibility := range eligibilityErrors {
		if errors.Is(err, eligibility) {
			httpx.Error(w, http.StatusForbidden, eligibility.Error())
			return
		}
	}
	switch {
	case errors.Is(err, domain.ErrInvalidUserID):
		httpx.ValidationFailed(w, "to_user_id", "must be a UUID")
	case errors.Is(err, domain.ErrSelfSwipe):
		httpx.ValidationFailed(w, "to_user_id", "must not be your own user ID")
	case errors.Is(err, domain.ErrCandidateUnavailable):
		httpx.Error(w, http.StatusNotFound, "candidate is unavailable")
	case errors.Is(err, domain.ErrConflict):
		httpx.Error(w, http.StatusConflict, "this user has already been swiped or matched")
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
