package media

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type Handler struct {
	service appmedia.Service
}

func NewHandler(service appmedia.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	command, err := readUpload(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	command.UserID = userID
	photo, err := h.service.Upload(r.Context(), command)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, uploadResponse{ID: photo.ID, URL: photo.URL, Position: photo.Position})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	photoID := r.PathValue("photoID")
	parsed, err := uuid.Parse(photoID)
	if err != nil || parsed.String() != strings.ToLower(photoID) {
		httpx.Error(w, http.StatusBadRequest, "invalid photo ID")
		return
	}
	if h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.service.Delete(r.Context(), userID, parsed.String()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	var bodyTooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, appmedia.ErrCleanupFailed):
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	case errors.As(err, &bodyTooLarge), errors.Is(err, appmedia.ErrPhotoTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "photo or request exceeds the size limit")
	case errors.Is(err, errInvalidMultipart):
		httpx.Error(w, http.StatusBadRequest, "expected one file and one position in a valid multipart body")
	case errors.Is(err, errUnsupportedContentType), errors.Is(err, appmedia.ErrUnsupportedPhoto):
		httpx.Error(w, http.StatusUnsupportedMediaType, "expected multipart JPEG, PNG, or WebP within the image processing limit")
	case errors.Is(err, appmedia.ErrInvalidPosition):
		httpx.ValidationFailed(w, "position", "must be an integer between 1 and 5")
	case errors.Is(err, appmedia.ErrPositionOccupied):
		httpx.Error(w, http.StatusConflict, "photo position is occupied")
	case errors.Is(err, appmedia.ErrProfileRequired):
		httpx.ErrorCode(w, http.StatusConflict, "profile_required", "create a profile before uploading photos")
	case errors.Is(err, appmedia.ErrPhotoNotFound):
		httpx.Error(w, http.StatusNotFound, "photo not found")
	case errors.Is(err, appmedia.ErrInvalidPhotoID):
		httpx.Error(w, http.StatusBadRequest, "invalid photo ID")
	case errors.Is(err, appmedia.ErrInvalidUserID):
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
