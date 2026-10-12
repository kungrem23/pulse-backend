package matching

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/swipes", requireUser(http.HandlerFunc(handler.Swipe)))
	mux.Handle("GET /api/v1/matches", requireUser(http.HandlerFunc(handler.List)))
	mux.Handle("DELETE /api/v1/matches/{matchID}", requireUser(http.HandlerFunc(handler.Close)))
}
