package middleware

import (
	"net/http"
	"strings"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

// JSONErrors replaces the plain-text 404 and 405 responses of http.ServeMux
// with the common JSON error envelope. The Allow header of a 405 is kept.
// Handlers that already answer in JSON are not affected.
func JSONErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&plainErrorWriter{ResponseWriter: w}, r)
	})
}

type plainErrorWriter struct {
	http.ResponseWriter
	replaced    bool
	wroteHeader bool
}

func (w *plainErrorWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	plain := strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain")
	if plain && (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) {
		w.replaced = true
		w.Header().Del("Content-Length")
		w.Header().Del("X-Content-Type-Options")
		message := "route not found"
		if code == http.StatusMethodNotAllowed {
			message = "method not allowed"
		}
		httpx.Error(w.ResponseWriter, code, message)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *plainErrorWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.replaced {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}

func (w *plainErrorWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
