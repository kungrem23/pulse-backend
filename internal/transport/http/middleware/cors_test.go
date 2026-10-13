package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const frontend = "https://pulse.example"

func TestParseOrigins(t *testing.T) {
	got, err := ParseOrigins(" https://Pulse.example , http://localhost:5173,,https://pulse.example ")
	want := []string{"https://pulse.example", "http://localhost:5173"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v, %v", want, got, err)
	}
	if got, err := ParseOrigins(""); err != nil || len(got) != 0 {
		t.Fatalf("empty value must disable CORS, got %v, %v", got, err)
	}
	for _, value := range []string{
		"*", "https://*.pulse.example", "pulse.example", "ftp://pulse.example", "https://",
		"https://pulse.example/", "https://pulse.example/app", "https://pulse.example?x=1",
		"https://user:pass@pulse.example", "null", "https://pulse.example#",
	} {
		if _, err := ParseOrigins(value); err == nil {
			t.Fatalf("%q must be rejected", value)
		}
	}
}

func corsHandler(origins ...string) (http.Handler, *int) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	return CORS(origins)(next), &calls
}

func corsRequest(handler http.Handler, method, origin string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1/matches", nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestCORSPreflight(t *testing.T) {
	handler, calls := corsHandler(frontend)
	response := corsRequest(handler, http.MethodOptions, frontend, map[string]string{
		"Access-Control-Request-Method":  "PATCH",
		"Access-Control-Request-Headers": "authorization, content-type",
	})
	header := response.Header()
	if response.Code != http.StatusNoContent || *calls != 0 {
		t.Fatalf("preflight must be answered without the next handler, got %d, calls %d", response.Code, *calls)
	}
	if header.Get("Access-Control-Allow-Origin") != frontend || header.Get("Access-Control-Allow-Credentials") != "" ||
		!strings.Contains(header.Get("Access-Control-Allow-Methods"), "PATCH") ||
		!strings.Contains(header.Get("Access-Control-Allow-Headers"), "Authorization") ||
		header.Get("Access-Control-Max-Age") == "" || !strings.Contains(strings.Join(header.Values("Vary"), ","), "Origin") {
		t.Fatalf("unexpected preflight headers %v", header)
	}
}

func TestCORSRejectsPreflight(t *testing.T) {
	handler, calls := corsHandler(frontend)
	for name, test := range map[string]struct {
		origin  string
		headers map[string]string
	}{
		"other origin":   {"https://evil.example", map[string]string{"Access-Control-Request-Method": "GET"}},
		"null origin":    {"null", map[string]string{"Access-Control-Request-Method": "GET"}},
		"method":         {frontend, map[string]string{"Access-Control-Request-Method": "TRACE"}},
		"custom header":  {frontend, map[string]string{"Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "x-admin"}},
		"subdomain trap": {"https://pulse.example.evil.example", map[string]string{"Access-Control-Request-Method": "GET"}},
	} {
		t.Run(name, func(t *testing.T) {
			response := corsRequest(handler, http.MethodOptions, test.origin, test.headers)
			if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" || *calls != 0 {
				t.Fatalf("expected 403 without CORS headers, got %d %v", response.Code, response.Header())
			}
			if body := decodeEnvelope(t, response); body.Error.Code != "forbidden" {
				t.Fatalf("unexpected envelope %+v", body)
			}
		})
	}
}

func TestCORSActualRequests(t *testing.T) {
	handler, calls := corsHandler(frontend)

	allowed := corsRequest(handler, http.MethodGet, frontend, nil)
	if allowed.Header().Get("Access-Control-Allow-Origin") != frontend ||
		!strings.Contains(allowed.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID") {
		t.Fatalf("allowed origin must get CORS headers, got %v", allowed.Header())
	}

	other := corsRequest(handler, http.MethodGet, "https://evil.example", nil)
	if other.Code != http.StatusOK || other.Header().Get("Access-Control-Allow-Origin") != "" || other.Header().Get("Vary") != "Origin" {
		t.Fatalf("other origins are served without CORS headers, got %d %v", other.Code, other.Header())
	}

	sameOrigin := corsRequest(handler, http.MethodGet, "", nil)
	if sameOrigin.Header().Get("Access-Control-Allow-Origin") != "" || sameOrigin.Header().Get("Vary") != "" {
		t.Fatalf("requests without Origin need no CORS headers, got %v", sameOrigin.Header())
	}

	// OPTIONS without Access-Control-Request-Method is not a preflight.
	corsRequest(handler, http.MethodOptions, frontend, nil)
	if *calls != 4 {
		t.Fatalf("expected 4 requests to reach the handler, got %d", *calls)
	}
}

func TestCORSDisabledWithoutOrigins(t *testing.T) {
	handler, calls := corsHandler()
	response := corsRequest(handler, http.MethodOptions, frontend, map[string]string{"Access-Control-Request-Method": "GET"})
	if response.Header().Get("Access-Control-Allow-Origin") != "" || *calls != 1 {
		t.Fatalf("without origins CORS must be a no-op, got %v, calls %d", response.Header(), *calls)
	}
}
