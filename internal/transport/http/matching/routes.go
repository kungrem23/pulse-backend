package matching

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("POST /api/v1/swipes", handler.Swipe)
	mux.HandleFunc("GET /api/v1/matches", handler.List)
	mux.HandleFunc("DELETE /api/v1/matches/{matchID}", handler.Close)
}
