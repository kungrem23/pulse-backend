package discovery

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/feed", requireUser(http.HandlerFunc(handler.Feed)))
}
