package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

const maxRequestIDLength = 64

type requestIDKey struct{}

// RequestID gives every request an ID: a client-supplied X-Request-ID is kept
// when it is 1-64 characters of [A-Za-z0-9._-], otherwise a random one is
// generated. The ID is returned in the X-Request-ID response header, put into
// error envelopes by httpx and available through RequestIDFrom.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(httpx.RequestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(httpx.RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// RequestIDFrom returns the ID set by RequestID, or "" outside of it.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// validRequestID keeps log lines and headers free of injected characters.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, char := range id {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9',
			char == '.', char == '_', char == '-':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var buffer [16]byte
	_, _ = rand.Read(buffer[:])
	return hex.EncodeToString(buffer[:])
}
