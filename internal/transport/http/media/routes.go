package media

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler) {
	mux.HandleFunc("POST /api/v1/photos", handler.Upload)
	mux.HandleFunc("DELETE /api/v1/photos/{photoID}", handler.Delete)
}
