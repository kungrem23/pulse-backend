package moderation

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("POST /api/v1/reports", handler.CreateReport)
	mux.HandleFunc("GET /api/v1/admin/reports", handler.ListReports)
	mux.HandleFunc("PATCH /api/v1/admin/reports/{reportID}", handler.ProcessReport)
	mux.HandleFunc("GET /api/v1/admin/users/{userID}", handler.InspectUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/block", handler.BlockUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/messages", handler.MessageUser)
	mux.HandleFunc("POST /api/v1/admin/users/{userID}/notifications", handler.NotifyUser)
}
