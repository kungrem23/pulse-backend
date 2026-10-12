package auth

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.HandleFunc("POST /api/v1/auth/register", handler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", handler.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", handler.Refresh)
	mux.Handle("POST /api/v1/auth/logout", requireUser(http.HandlerFunc(handler.Logout)))
	mux.HandleFunc("POST /api/v1/admin/auth/login", handler.LoginAdmin)
}
