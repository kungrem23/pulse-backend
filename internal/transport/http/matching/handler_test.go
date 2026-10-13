package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmatching "github.com/kungrem23/pulse-backend/internal/app/matching"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

const (
	callerID = "11111111-1111-4111-8111-111111111111"
	targetID = "22222222-2222-4222-8222-222222222222"
	matchID  = "33333333-3333-4333-8333-333333333333"
)

type authenticatorStub struct{}

func (authenticatorStub) Authenticate(context.Context, string) (string, error) {
	return callerID, nil
}

type serviceStub struct {
	match   *domain.Match
	err     error
	calls   int
	command appmatching.SwipeCommand

	page        domain.Page[appmatching.MatchSummary]
	listCalls   int
	listUser    string
	listRequest domain.PageRequest
}

func (s *serviceStub) Swipe(_ context.Context, command appmatching.SwipeCommand) (*domain.Match, error) {
	s.calls++
	s.command = command
	return s.match, s.err
}

func (s *serviceStub) List(_ context.Context, userID string, request domain.PageRequest) (domain.Page[appmatching.MatchSummary], error) {
	s.listCalls++
	s.listUser, s.listRequest = userID, request
	return s.page, s.err
}

func (s *serviceStub) Close(context.Context, string, string) error {
	return errors.New("unexpected call")
}

func serve(service appmatching.Service, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service), middleware.RequireUser(authenticatorStub{}))
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func swipe(service appmatching.Service, body string) *httptest.ResponseRecorder {
	return serve(service, http.MethodPost, "/api/v1/swipes", body)
}

type errorBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	} `json:"error"`
}

func decodeError(t *testing.T, response *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", response.Body, err)
	}
	return body
}

func TestSwipeWithoutMatch(t *testing.T) {
	service := &serviceStub{}
	response := swipe(service, `{"to_user_id":"`+targetID+`","is_like":true}`)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"matched":false}` {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body)
	}
	want := appmatching.SwipeCommand{FromUserID: callerID, ToUserID: targetID, IsLike: true}
	if service.command != want {
		t.Fatalf("unexpected command %+v", service.command)
	}
}

func TestSwipeWithMatch(t *testing.T) {
	service := &serviceStub{match: &domain.Match{ID: matchID, Status: "active"}}
	response := swipe(service, `{"to_user_id":"`+targetID+`","is_like":true}`)

	want := `{"matched":true,"match_id":"` + matchID + `"}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body)
	}
}

func TestSwipeDislike(t *testing.T) {
	service := &serviceStub{}
	response := swipe(service, `{"to_user_id":"`+targetID+`","is_like":false}`)
	if response.Code != http.StatusOK || service.calls != 1 || service.command.IsLike {
		t.Fatalf("dislike must be passed as is_like=false: %d %+v", response.Code, service.command)
	}
}

func TestSwipeIgnoresCallerFromBody(t *testing.T) {
	service := &serviceStub{}
	response := swipe(service, `{"from_user_id":"`+targetID+`","to_user_id":"`+targetID+`","is_like":true}`)
	if response.Code != http.StatusBadRequest || service.calls != 0 {
		t.Fatalf("unknown fields must be rejected, got %d", response.Code)
	}
}

func TestSwipeRejectsMalformedBody(t *testing.T) {
	for name, body := range map[string]string{
		"empty":           ``,
		"missing is_like": `{"to_user_id":"` + targetID + `"}`,
		"null is_like":    `{"to_user_id":"` + targetID + `","is_like":null}`,
		"string is_like":  `{"to_user_id":"` + targetID + `","is_like":"true"}`,
		"numeric target":  `{"to_user_id":1,"is_like":true}`,
		"two values":      `{"to_user_id":"` + targetID + `","is_like":true}{}`,
		"array":           `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			service := &serviceStub{}
			response := swipe(service, body)
			if response.Code != http.StatusBadRequest || decodeError(t, response).Error.Code != "bad_request" || service.calls != 0 {
				t.Fatalf("expected 400 without a call, got %d: %s", response.Code, response.Body)
			}
		})
	}
}

func TestSwipeMapsErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
		field  string
	}{
		{domain.ErrInvalidUserID, http.StatusUnprocessableEntity, "validation_failed", "to_user_id"},
		{domain.ErrSelfSwipe, http.StatusUnprocessableEntity, "validation_failed", "to_user_id"},
		{domain.ErrBlocked, http.StatusForbidden, "forbidden", ""},
		{domain.ErrProfileIncomplete, http.StatusForbidden, "forbidden", ""},
		{domain.ErrNoPhotos, http.StatusForbidden, "forbidden", ""},
		{domain.ErrSleeping, http.StatusForbidden, "forbidden", ""},
		{domain.ErrCandidateUnavailable, http.StatusNotFound, "not_found", ""},
		{domain.ErrConflict, http.StatusConflict, "conflict", ""},
		{domain.ErrNotFound, http.StatusUnauthorized, "unauthorized", ""},
		{errors.New("pq: relation swipes does not exist"), http.StatusInternalServerError, "internal_error", ""},
	} {
		t.Run(test.err.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("swipe: save: %w", test.err)
			response := swipe(&serviceStub{err: wrapped}, `{"to_user_id":"`+targetID+`","is_like":true}`)
			body := decodeError(t, response)
			if response.Code != test.status || body.Error.Code != test.code {
				t.Fatalf("expected %d %s, got %d: %s", test.status, test.code, response.Code, response.Body)
			}
			if test.field != "" && body.Error.Details[test.field] == "" {
				t.Fatalf("expected details for %s: %s", test.field, response.Body)
			}
			if strings.Contains(response.Body.String(), "swipe: save") || strings.Contains(response.Body.String(), "pq:") {
				t.Fatalf("internal error text leaked: %s", response.Body)
			}
		})
	}
}

func TestSwipeWithoutServiceIsInternalError(t *testing.T) {
	response := swipe(nil, `{"to_user_id":"`+targetID+`","is_like":true}`)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}

func TestSwipeRequiresAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	service := &serviceStub{}
	RegisterRoutes(mux, NewHandler(service), middleware.RequireUser(authenticatorStub{}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/swipes", strings.NewReader(`{"to_user_id":"`+targetID+`","is_like":true}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || service.calls != 0 {
		t.Fatalf("expected 401 without a call, got %d", response.Code)
	}
}

func TestCloseMatchStaysNotImplemented(t *testing.T) {
	response := serve(&serviceStub{}, http.MethodDelete, "/api/v1/matches/"+matchID, "")
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", response.Code)
	}
}

func TestListMatches(t *testing.T) {
	createdAt := time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC)
	service := &serviceStub{page: domain.Page[appmatching.MatchSummary]{
		Items: []appmatching.MatchSummary{
			{ID: matchID, Status: "active", CreatedAt: createdAt, User: appmatching.PublicUserSummary{
				ID: targetID, Name: "Анна", Age: 24, PhotoURL: "https://photos.example/1",
			}},
			{ID: targetID, Status: "closed", CreatedAt: createdAt, User: appmatching.PublicUserSummary{
				ID: callerID, Name: "Борис", Age: 30,
			}},
		},
		NextCursor: "next",
	}}
	response := serve(service, http.MethodGet, "/api/v1/matches?limit=2&cursor=abc", "")

	want := `{"items":[` +
		`{"id":"` + matchID + `","status":"active","created_at":"2026-10-13T12:00:00.123456Z","user":{"id":"` + targetID + `","name":"Анна","age":24,"photo_url":"https://photos.example/1"}},` +
		`{"id":"` + targetID + `","status":"closed","created_at":"2026-10-13T12:00:00.123456Z","user":{"id":"` + callerID + `","name":"Борис","age":30}}` +
		`],"next_cursor":"next"}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("unexpected response %d:\n%s\nwant:\n%s", response.Code, response.Body, want)
	}
	if service.listUser != callerID || service.listRequest != (domain.PageRequest{Cursor: "abc", Limit: 2}) {
		t.Fatalf("unexpected list call %s %+v", service.listUser, service.listRequest)
	}
}

func TestListMatchesEmptyPage(t *testing.T) {
	for name, page := range map[string]domain.Page[appmatching.MatchSummary]{
		"nil items":   {},
		"empty items": {Items: []appmatching.MatchSummary{}},
	} {
		t.Run(name, func(t *testing.T) {
			response := serve(&serviceStub{page: page}, http.MethodGet, "/api/v1/matches", "")
			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"items":[]}` {
				t.Fatalf("unexpected response %d: %s", response.Code, response.Body)
			}
		})
	}
}

func TestListMatchesRejectsQueryBeforeService(t *testing.T) {
	for _, query := range []string{"?limit=0", "?limit=abc", "?limit=-1", "?cursor=", "?cursor=a&cursor=b", "?limit=1&limit=2"} {
		service := &serviceStub{}
		response := serve(service, http.MethodGet, "/api/v1/matches"+query, "")
		if response.Code != http.StatusBadRequest || decodeError(t, response).Error.Code != "bad_request" || service.listCalls != 0 {
			t.Fatalf("%s: expected 400 without a call, got %d: %s", query, response.Code, response.Body)
		}
	}
}

func TestListMatchesMapsErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidCursor, http.StatusBadRequest, "bad_request"},
		{domain.ErrInvalidLimit, http.StatusBadRequest, "bad_request"},
		{domain.ErrNotFound, http.StatusUnauthorized, "unauthorized"},
		{errors.New("match 1 does not belong to the caller"), http.StatusInternalServerError, "internal_error"},
	} {
		response := serve(&serviceStub{err: fmt.Errorf("list matches: %w", test.err)}, http.MethodGet, "/api/v1/matches", "")
		if response.Code != test.status || decodeError(t, response).Error.Code != test.code {
			t.Fatalf("%v: expected %d %s, got %d: %s", test.err, test.status, test.code, response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), "list matches") || strings.Contains(response.Body.String(), "belong") {
			t.Fatalf("internal error text leaked: %s", response.Body)
		}
	}
}

func TestListMatchesWithoutServiceIsInternalError(t *testing.T) {
	response := serve(nil, http.MethodGet, "/api/v1/matches", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}
