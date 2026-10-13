package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

const (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE"
	corsAllowedHeaders = "Authorization, Content-Type, X-Request-ID"
	corsExposedHeaders = "X-Request-ID, Retry-After"
	corsMaxAgeSeconds  = "600"
)

var corsMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

var corsHeaders = map[string]bool{"authorization": true, "content-type": true, "x-request-id": true}

// ParseOrigins reads a comma-separated list such as
// "https://pulse.example,http://localhost:5173". Each origin must be an exact
// http(s) scheme://host[:port] without path, credentials or wildcard.
// An empty value gives an empty list, which disables CORS.
func ParseOrigins(value string) ([]string, error) {
	var origins []string
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		origin, err := normalizeOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("CORS origin %q: %w", raw, err)
		}
		if !seen[origin] {
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	return origins, nil
}

func normalizeOrigin(raw string) (string, error) {
	if strings.Contains(raw, "*") {
		return "", errors.New("wildcards are not allowed")
	}
	parsed, err := url.Parse(raw)
	switch {
	case err != nil:
		return "", errors.New("must be a URL")
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "", errors.New("scheme must be http or https")
	case parsed.Host == "" || parsed.Hostname() == "":
		return "", errors.New("host is required")
	case parsed.User != nil:
		return "", errors.New("credentials are not allowed")
	case parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasSuffix(raw, "?") || strings.HasSuffix(raw, "#"):
		return "", errors.New("path, query and fragment are not allowed")
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

// CORS allows browser requests only from the listed origins. Responses never
// allow credentials: the API uses Bearer tokens, not cookies. Preflight
// requests are answered here, before authentication. A disallowed preflight
// gets 403; a normal request from a disallowed origin is processed without
// CORS headers, so the browser does not expose the response.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if !allowed[strings.ToLower(origin)] {
				if preflight {
					httpx.Error(w, http.StatusForbidden, "origin is not allowed")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			if !preflight {
				w.Header().Set("Access-Control-Expose-Headers", corsExposedHeaders)
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			if !corsMethods[r.Header.Get("Access-Control-Request-Method")] || !allowedRequestHeaders(r) {
				w.Header().Del("Access-Control-Allow-Origin")
				httpx.Error(w, http.StatusForbidden, "CORS request is not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			w.Header().Set("Access-Control-Max-Age", corsMaxAgeSeconds)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func allowedRequestHeaders(r *http.Request) bool {
	for _, value := range r.Header.Values("Access-Control-Request-Headers") {
		for _, header := range strings.Split(value, ",") {
			name := strings.ToLower(strings.TrimSpace(header))
			if name != "" && !corsHeaders[name] {
				return false
			}
		}
	}
	return true
}
