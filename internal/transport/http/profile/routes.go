package profile

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/profile", requireUser(http.HandlerFunc(handler.Get)))
	mux.Handle("PUT /api/v1/profile", requireUser(http.HandlerFunc(handler.Update)))
	mux.Handle("PUT /api/v1/preferences", requireUser(http.HandlerFunc(handler.UpdatePreferences)))
	mux.Handle("PATCH /api/v1/profile/sleep", requireUser(http.HandlerFunc(handler.SetSleep)))
}
