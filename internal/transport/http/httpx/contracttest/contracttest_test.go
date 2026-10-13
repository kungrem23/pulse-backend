package contracttest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func recorder(status int, contentType, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	if contentType != "" {
		response.Header().Set("Content-Type", contentType)
	}
	response.WriteHeader(status)
	response.WriteString(body)
	return response
}

func TestSpecIsConsistent(t *testing.T) {
	spec := Load(t)
	if broken := spec.BrokenRefs(); len(broken) != 0 {
		t.Fatalf("unresolved $ref: %v", broken)
	}
	seen := map[string]string{}
	for route, id := range spec.Operations() {
		if id == "" {
			t.Fatalf("%s has no operationId", route)
		}
		if other, ok := seen[id]; ok {
			t.Fatalf("operationId %s is used by %s and %s", id, other, route)
		}
		seen[id] = route
	}
	if len(seen) == 0 {
		t.Fatal("no operations found")
	}
}

const (
	jsonCT = "application/json; charset=utf-8"
	id     = "11111111-1111-4111-8111-111111111111"
)

func TestValidateAcceptsContractResponses(t *testing.T) {
	spec := Load(t)
	for name, test := range map[string]struct {
		method, path string
		response     *httptest.ResponseRecorder
	}{
		"swipe":        {"POST", "/api/v1/swipes", recorder(200, jsonCT, `{"matched":true,"match_id":"`+id+`"}`)},
		"empty page":   {"GET", "/api/v1/matches", recorder(200, jsonCT, `{"items":[]}`)},
		"error":        {"GET", "/api/v1/matches", recorder(400, jsonCT, `{"error":{"code":"bad_request","message":"x"},"request_id":"r"}`)},
		"with details": {"POST", "/api/v1/swipes", recorder(422, jsonCT, `{"error":{"code":"validation_failed","message":"x","details":{"to_user_id":"bad"}}}`)},
		"match page": {"GET", "/api/v1/matches", recorder(200, jsonCT, `{"items":[{"id":"`+id+`","status":"closed",`+
			`"created_at":"2026-10-13T12:00:00.123456Z","user":{"id":"`+id+`","name":"A","age":18,"photo_url":"https://x/y"}}],"next_cursor":"c"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := spec.Validate(test.method, test.path, test.response); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateRejectsContractViolations(t *testing.T) {
	spec := Load(t)
	tooMany := recorder(429, jsonCT, `{"error":{"code":"too_many_requests","message":"x"}}`)
	for name, test := range map[string]struct {
		method, path string
		response     *httptest.ResponseRecorder
		want         string
	}{
		"unknown path":        {"GET", "/nope", recorder(200, jsonCT, `{}`), "not in OpenAPI"},
		"undeclared status":   {"POST", "/api/v1/swipes", recorder(201, jsonCT, `{"matched":false}`), "not declared"},
		"text body":           {"POST", "/api/v1/swipes", recorder(200, "text/plain", `{"matched":false}`), "Content-Type"},
		"missing required":    {"POST", "/api/v1/swipes", recorder(200, jsonCT, `{}`), "required"},
		"extra property":      {"POST", "/api/v1/swipes", recorder(200, jsonCT, `{"matched":false,"x":1}`), "not allowed"},
		"wrong type":          {"POST", "/api/v1/swipes", recorder(200, jsonCT, `{"matched":"no"}`), "expected type"},
		"bad uuid":            {"POST", "/api/v1/swipes", recorder(200, jsonCT, `{"matched":true,"match_id":"x"}`), "UUID"},
		"null items":          {"GET", "/api/v1/matches", recorder(200, jsonCT, `{"items":null}`), "expected type"},
		"bad enum":            {"GET", "/api/v1/matches", recorder(200, jsonCT, `{"items":[{"id":"`+id+`","status":"x","created_at":"2026-10-13T12:00:00Z","user":{"id":"`+id+`","name":"A"}}]}`), "is not one of"},
		"bad error code":      {"GET", "/api/v1/matches", recorder(400, jsonCT, `{"error":{"code":"Bad Request","message":"x"}}`), "does not match"},
		"missing Retry-After": {"POST", "/api/v1/auth/login", tooMany, "Retry-After"},
	} {
		t.Run(name, func(t *testing.T) {
			err := spec.Validate(test.method, test.path, test.response)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected an error containing %q, got %v", test.want, err)
			}
		})
	}

	tooMany.Header().Set("Retry-After", "3")
	if err := spec.Validate(http.MethodPost, "/api/v1/auth/login", tooMany); err != nil {
		t.Fatalf("429 with Retry-After must pass: %v", err)
	}
}
