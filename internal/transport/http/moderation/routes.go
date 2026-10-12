package moderation

import "net/http"

// Admin routes stay unauthenticated 501 placeholders until the moderation release.
func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/reports", requireUser(http.HandlerFunc(handler.CreateReport)))
	mux.HandleFunc("GET /api/v1/admin/reports", handler.ListReports)
	mux.HandleFunc("PATCH /api/v1/admin/reports/{reportID}", handler.ProcessReport)
	mux.HandleFunc("GET /api/v1/admin/users/{userID}", handler.InspectUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/block", handler.BlockUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/messages", handler.MessageUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/notifications", handler.NotifyUser)
}
