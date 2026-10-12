package chat

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/conversations", requireUser(http.HandlerFunc(handler.Conversations)))
	mux.Handle("GET /api/v1/conversations/{conversationID}/messages", requireUser(http.HandlerFunc(handler.List)))
	mux.Handle("POST /api/v1/conversations/{conversationID}/messages", requireUser(http.HandlerFunc(handler.Send)))
}
