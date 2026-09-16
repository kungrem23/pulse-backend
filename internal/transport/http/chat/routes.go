package chat

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("GET /api/v1/matches/{matchID}/messages", handler.List)
	mux.HandleFunc("POST /api/v1/matches/{matchID}/messages", handler.Send)
}
