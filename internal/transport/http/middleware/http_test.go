package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx"
)

func decodeEnvelope(t *testing.T, response *httptest.ResponseRecorder) httpx.ErrorResponse {
	t.Helper()
	var body httpx.ErrorResponse
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		t.Fatalf("expected a JSON error envelope, got %q: %v", response.Body, err)
	}
	return body
}

func TestRequestIDGeneratesAndPropagates(t *testing.T) {
	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
		httpx.Error(w, http.StatusConflict, "conflict")
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	id := response.Header().Get(httpx.RequestIDHeader)
	if len(id) != 32 || seen != id {
		t.Fatalf("expected a generated 32-char ID in header and context, got %q / %q", id, seen)
	}
	if body := decodeEnvelope(t, response); body.RequestID != id {
		t.Fatalf("error envelope must carry the request ID, got %+v", body)
	}

	other := httptest.NewRecorder()
	handler.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/", nil))
	if other.Header().Get(httpx.RequestIDHeader) == id {
		t.Fatal("request IDs must be unique")
	}
}

func TestRequestIDKeepsOnlySafeClientValues(t *testing.T) {
	for value, keep := range map[string]bool{
		"abc-123_X.y":           true,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
		"has space":             false,
		"inject\nline":          false,
		"quote\"":               false,
		"кириллица":             false,
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(httpx.RequestIDHeader, value)
		response := httptest.NewRecorder()
		RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(response, request)
		got := response.Header().Get(httpx.RequestIDHeader)
		if (got == value) != keep || got == "" {
			t.Fatalf("%q: keep=%v, got %q", value, keep, got)
		}
	}
}

func TestLoggingRecordsStatusAndRequestIDWithoutQuery(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := RequestID(Logging(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusTeapot, "teapot")
	})))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/matches?cursor=secret-cursor", nil)
	request.Header.Set(httpx.RequestIDHeader, "req-7")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("decode log %q: %v", logs.String(), err)
	}
	if line["status"] != float64(http.StatusTeapot) || line["request_id"] != "req-7" || line["path"] != "/api/v1/matches" {
		t.Fatalf("unexpected log line %v", line)
	}
	if strings.Contains(logs.String(), "secret-cursor") {
		t.Fatalf("query string must not be logged: %s", logs.String())
	}
}

func TestLoggingDefaultsToOK(t *testing.T) {
	var logs bytes.Buffer
	handler := Logging(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(logs.String(), `"status":200`) {
		t.Fatalf("expected status 200 in %s", logs.String())
	}
}

func TestJSONErrorsWrapsServeMuxErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/swipes", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"matched": false})
	})
	mux.HandleFunc("GET /api/v1/missing", func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusNotFound, "profile not found")
	})
	handler := RequestID(JSONErrors(mux))

	serve := func(method, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}

	notFound := serve(http.MethodGet, "/unknown")
	if body := decodeEnvelope(t, notFound); notFound.Code != http.StatusNotFound || body.Error.Code != "not_found" || body.RequestID == "" {
		t.Fatalf("unexpected 404 %d: %+v", notFound.Code, body)
	}
	if !strings.HasPrefix(notFound.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("404 must be JSON, got %q", notFound.Header().Get("Content-Type"))
	}

	notAllowed := serve(http.MethodGet, "/api/v1/swipes")
	if body := decodeEnvelope(t, notAllowed); notAllowed.Code != http.StatusMethodNotAllowed || body.Error.Code != "method_not_allowed" {
		t.Fatalf("unexpected 405 %d: %+v", notAllowed.Code, body)
	}
	if notAllowed.Header().Get("Allow") == "" {
		t.Fatal("405 must keep the Allow header")
	}

	handled := serve(http.MethodGet, "/api/v1/missing")
	if body := decodeEnvelope(t, handled); body.Error.Message != "profile not found" {
		t.Fatalf("a handler's own JSON 404 must be kept, got %+v", body)
	}
	if ok := serve(http.MethodPost, "/api/v1/swipes"); ok.Code != http.StatusOK || strings.TrimSpace(ok.Body.String()) != `{"matched":false}` {
		t.Fatalf("successful responses must pass through, got %d %s", ok.Code, ok.Body)
	}
}
