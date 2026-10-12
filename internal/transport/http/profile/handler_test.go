package profile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

type authenticatorStub struct{}

func (authenticatorStub) Authenticate(context.Context, string) (string, error) {
	return "user-from-token", nil
}

type serviceStub struct {
	view    appprofile.View
	getErr  error
	profile domain.Profile
	err     error
	command appprofile.UpdateCommand
	getUser string
}

func (s *serviceStub) Get(_ context.Context, userID string) (appprofile.View, error) {
	s.getUser = userID
	return s.view, s.getErr
}

func (s *serviceStub) Update(_ context.Context, command appprofile.UpdateCommand) (domain.Profile, error) {
	s.command = command
	return s.profile, s.err
}

func serve(service appprofile.Service, method, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service), middleware.RequireUser(authenticatorStub{}))
	request := httptest.NewRequest(method, "/api/v1/profile", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestGetWithoutPreferencesReturnsNullAndEmptyPhotos(t *testing.T) {
	service := &serviceStub{view: appprofile.View{
		Profile: domain.Profile{UserID: "user-from-token", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"},
	}}
	response := serve(service, http.MethodGet, "")

	want := `{"profile":{"user_id":"user-from-token","name":"Анна","bio":"","age":25,"sex":"female","city":"Москва","is_sleeping":false},"preferences":null,"photos":[]}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body)
	}
	if service.getUser != "user-from-token" {
		t.Fatalf("the user must come from the access token, got %q", service.getUser)
	}
}

func TestGetWithPreferences(t *testing.T) {
	service := &serviceStub{view: appprofile.View{
		Profile:     domain.Profile{UserID: "u", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"},
		Preferences: &domain.Preferences{UserID: "u", MinAge: 20, MaxAge: 30, Sex: domain.SearchSexAny},
	}}
	response := serve(service, http.MethodGet, "")
	if !strings.Contains(response.Body.String(), `"preferences":{"user_id":"u","min_age":20,"max_age":30,"sex":"any"}`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestGetMapsErrors(t *testing.T) {
	if response := serve(&serviceStub{getErr: appprofile.ErrProfileNotFound}, http.MethodGet, ""); response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
	if response := serve(&serviceStub{getErr: errors.New("down")}, http.MethodGet, ""); response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}

func TestUpdatePassesFieldsAndUserFromToken(t *testing.T) {
	service := &serviceStub{profile: domain.Profile{UserID: "user-from-token", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва", IsSleeping: true}}
	response := serve(service, http.MethodPut, `{"name":"Анна","age":25,"sex":"female","city":"Москва","bio":"Привет"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body)
	}
	want := appprofile.UpdateCommand{UserID: "user-from-token", Name: "Анна", Bio: "Привет", Age: 25, Sex: "female", City: "Москва"}
	if service.command != want {
		t.Fatalf("unexpected command %+v", service.command)
	}
	if !strings.Contains(response.Body.String(), `"is_sleeping":true`) {
		t.Fatalf("response must reflect the stored profile: %s", response.Body)
	}
}

func TestUpdateRejectsClientSuppliedOwner(t *testing.T) {
	service := &serviceStub{}
	response := serve(service, http.MethodPut, `{"user_id":"someone-else","name":"Анна","age":25,"sex":"female","city":"Москва"}`)
	if response.Code != http.StatusBadRequest || service.command.UserID != "" {
		t.Fatalf("unknown fields such as user_id must be rejected, got %d", response.Code)
	}
}

func TestUpdateMapsErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"validation", `{"name":"","age":25,"sex":"female","city":"Москва"}`, &appprofile.ValidationError{Field: "name", Message: "must contain 1 to 100 characters"}, http.StatusUnprocessableEntity, `"details":{"name":"must contain 1 to 100 characters"}`},
		{"malformed json", `{"name":`, nil, http.StatusBadRequest, `"bad_request"`},
		{"string age", `{"name":"А","age":"25","sex":"female","city":"М"}`, nil, http.StatusBadRequest, `"bad_request"`},
		{"user vanished", `{"name":"А","age":25,"sex":"female","city":"М"}`, appprofile.ErrUserNotFound, http.StatusUnauthorized, `"unauthorized"`},
		{"storage failure", `{"name":"А","age":25,"sex":"female","city":"М"}`, errors.New("down"), http.StatusInternalServerError, `"internal_error"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serve(&serviceStub{err: test.err}, http.MethodPut, test.body)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("expected %d with %s, got %d: %s", test.wantStatus, test.wantBody, response.Code, response.Body)
			}
		})
	}
}
