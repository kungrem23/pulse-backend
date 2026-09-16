package health

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("GET /health/live", handler.Live)
	mux.HandleFunc("GET /health/ready", handler.Ready)
}
