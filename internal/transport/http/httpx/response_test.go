package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
