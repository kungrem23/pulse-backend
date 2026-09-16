package profile

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("GET /api/v1/profile", handler.Get)
	mux.HandleFunc("PUT /api/v1/profile", handler.Update)
	mux.HandleFunc("PUT /api/v1/preferences", handler.UpdatePreferences)
	mux.HandleFunc("PATCH /api/v1/profile/sleep", handler.SetSleep)
}
