package media

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/photos", requireUser(http.HandlerFunc(handler.Upload)))
	mux.Handle("DELETE /api/v1/photos/{photoID}", requireUser(http.HandlerFunc(handler.Delete)))
}
