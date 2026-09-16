package discovery

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("GET /api/v1/feed", handler.Feed)
}
