package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorsUseOpenAPIEnvelope(t *testing.T) {
	res := httptest.NewRecorder()
	ErrorCode(res, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
	var body ErrorResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if res.Code != http.StatusUnauthorized || body.Error.Code != "invalid_credentials" || body.Error.Message == "" {
		t.Fatal("expected OpenAPI error.code and error.message")
	}
}

func TestPlaceholderUsesErrorDetails(t *testing.T) {
	res := httptest.NewRecorder()
	NotImplemented("rotate refresh session")(res, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil))
	var body ErrorResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode placeholder: %v", err)
	}
	if res.Code != http.StatusNotImplemented || body.Error.Code != "not_implemented" || body.Error.Details["operation"] == "" {
		t.Fatal("expected 501 in the same OpenAPI error envelope")
	}
}

func TestErrorIncludesRequestID(t *testing.T) {
	res := httptest.NewRecorder()
	res.Header().Set(RequestIDHeader, "req-1")
	Error(res, http.StatusNotFound, "not found")
	var body ErrorResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID != "req-1" || body.Error.Code != "not_found" {
		t.Fatalf("expected request_id in the envelope, got %s", res.Body)
	}

	res = httptest.NewRecorder()
	Error(res, http.StatusMethodNotAllowed, "method not allowed")
	if strings.Contains(res.Body.String(), "request_id") || !strings.Contains(res.Body.String(), `"method_not_allowed"`) {
		t.Fatalf("unexpected body without request ID: %s", res.Body)
	}
}

type decodeTarget struct {
	Name string `json:"name"`
}

func decodeRequest(contentType, body string) (decodeTarget, error) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	var target decodeTarget
	err := DecodeJSON(request, &target)
	return target, err
}

func TestDecodeJSONContentType(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "Application/JSON", "application/json; charset=utf-8", "application/json;charset=UTF-8"} {
		if got, err := decodeRequest(contentType, `{"name":"a"}`); err != nil || got.Name != "a" {
			t.Fatalf("%q must be accepted: %+v, %v", contentType, got, err)
		}
	}
	for _, contentType := range []string{
		"text/plain", "text/plain;charset=UTF-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x",
		"application/json; charset=latin1", "application/jsonx", "application/problem+json", "garbage;;",
	} {
		if _, err := decodeRequest(contentType, `{"name":"a"}`); !errors.Is(err, ErrUnsupportedContentType) {
			t.Fatalf("%q must be rejected, got %v", contentType, err)
		}
	}
}

func TestDecodeJSONStrictBody(t *testing.T) {
	for name, body := range map[string]string{
		"empty":         ``,
		"unknown field": `{"name":"a","admin":true}`,
		"two values":    `{"name":"a"} {"name":"b"}`,
		"trailing junk": `{"name":"a"} x`,
		"wrong type":    `{"name":1}`,
		"array":         `[]`,
	} {
		if _, err := decodeRequest("application/json", body); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	if _, err := decodeRequest("", `{"name":"a"}`+"\n  "); err != nil {
		t.Fatalf("trailing whitespace must be accepted: %v", err)
	}
}

func TestDecodeJSONBodyLimit(t *testing.T) {
	padding := strings.Repeat(" ", MaxJSONBodyBytes-len(`{"name":"a"}`))
	if _, err := decodeRequest("", `{"name":"a"}`+padding); err != nil {
		t.Fatalf("a body of exactly the limit must be accepted: %v", err)
	}
	if _, err := decodeRequest("", `{"name":"a"}`+padding+" "); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge for trailing bytes over the limit, got %v", err)
	}
	long := `{"name":"` + strings.Repeat("a", MaxJSONBodyBytes) + `"}`
	if _, err := decodeRequest("", long); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}
