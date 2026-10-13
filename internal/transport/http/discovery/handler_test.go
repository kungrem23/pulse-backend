package discovery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	"github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

const ownerID = "11111111-1111-4111-8111-111111111111"

type serviceStub struct {
	result domain.Page[appdiscovery.Candidate]
	err    error
	owner  string
	page   domain.PageRequest
	calls  int
}

func (s *serviceStub) Feed(_ context.Context, owner string, page domain.PageRequest) (domain.Page[appdiscovery.Candidate], error) {
	s.calls++
	s.owner, s.page = owner, page
	return s.result, s.err
}

type authenticatorStub struct{}

func (authenticatorStub) Authenticate(_ context.Context, token string) (string, error) {
	if token == "blocked" {
		return "", appauth.ErrUserBlocked
	}
	if token != "valid" {
		return "", appauth.ErrInvalidAccessToken
	}
	return ownerID, nil
}

func serve(service appdiscovery.Service, rawQuery, token string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service), middleware.RequireUser(authenticatorStub{}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	request.URL.RawQuery = rawQuery
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestFeedDTOAndVerifiedOwner(t *testing.T) {
	service := &serviceStub{result: domain.Page[appdiscovery.Candidate]{
		Items: []appdiscovery.Candidate{{
			Profile: domain.Profile{UserID: "22222222-2222-4222-8222-222222222222", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Казань", Bio: "Привет"},
			Photos:  []media.Photo{{ID: "33333333-3333-4333-8333-333333333333", URL: "https://media.example.test/photo", Position: 2}},
		}}, NextCursor: "next-cursor",
	}}
	response := serve(service, "limit=1&cursor="+url.QueryEscape("opaque-cursor"), "valid")
	if response.Code != 200 || service.owner != ownerID || service.page.Limit != 1 || service.page.Cursor != "opaque-cursor" {
		t.Fatalf("unexpected request: %d %+v", response.Code, service)
	}
	for _, field := range []string{`"user_id":"22222222-2222-4222-8222-222222222222"`, `"name":"Анна"`, `"age":25`, `"bio":"Привет"`, `"sex":"female"`, `"city":"Казань"`, `"photos":[{"id":"33333333-3333-4333-8333-333333333333","url":"https://media.example.test/photo","position":2}]`, `"next_cursor":"next-cursor"`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("missing %s: %s", field, response.Body)
		}
	}
	if strings.Contains(response.Body.String(), "object_key") || strings.Contains(response.Body.String(), "is_sleeping") {
		t.Fatal("internal fields leaked")
	}
	service.result = domain.Page[appdiscovery.Candidate]{}
	response = serve(service, "", "valid")
	if strings.TrimSpace(response.Body.String()) != `{"items":[]}` || service.page.Limit != 20 {
		t.Fatalf("empty/default response: %s", response.Body)
	}
}

func TestFeedRejectsBadQueryBeforeService(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=-1", "limit=101", "limit=1.5", "limit=", "limit=huge", "limit=1&limit=2", "cursor=", "cursor=a&cursor=b", "cursor=" + strings.Repeat("a", 513), "limit=%zz", "limit=1;cursor=x", "user_id=foreign", "city=Москва"} {
		service := &serviceStub{}
		response := serve(service, query, "valid")
		if response.Code != 400 || service.calls != 0 {
			t.Fatalf("query %q: expected 400 before service, got %d", query, response.Code)
		}
	}
	service := &serviceStub{}
	if response := serve(service, "limit=100", "valid"); response.Code != 200 || service.page.Limit != 100 {
		t.Fatalf("max limit rejected: %d", response.Code)
	}
}

func TestFeedMapsErrorsAndProtectsRoute(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidCursor, 400, "bad_request"}, {domain.ErrInvalidLimit, 400, "bad_request"},
		{domain.ErrInvalidUserID, 401, "unauthorized"}, {domain.ErrNotFound, 401, "unauthorized"},
		{domain.ErrProfileIncomplete, 403, "forbidden"}, {domain.ErrNoPhotos, 403, "forbidden"},
		{domain.ErrSleeping, 403, "forbidden"}, {domain.ErrBlocked, 403, "forbidden"},
		{errors.New("SECRET SQL/S3 object_key"), 500, "internal_error"},
	} {
		response := serve(&serviceStub{err: errors.Join(errors.New("operation"), test.err)}, "", "valid")
		if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) || strings.Contains(response.Body.String(), "SECRET") {
			t.Fatalf("error %v: expected %d/%s, got %d %s", test.err, test.status, test.code, response.Code, response.Body)
		}
	}
	for _, token := range []string{"", "refresh", "admin", "blocked"} {
		service := &serviceStub{}
		response := serve(service, "", token)
		status := 401
		if token == "blocked" {
			status = 403
		}
		if response.Code != status || service.calls != 0 {
			t.Fatalf("token %s bypassed auth: %d", token, response.Code)
		}
	}
	if response := serve(nil, "", "valid"); response.Code != 500 {
		t.Fatalf("nil service: %d", response.Code)
	}
}
