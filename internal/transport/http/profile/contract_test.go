package profile

import (
	"errors"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/app/media"
	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

func TestProfileResponsesMatchOpenAPI(t *testing.T) {
	spec := contracttest.Load(t)
	profile := domain.Profile{UserID: "11111111-1111-4111-8111-111111111111", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}
	view := appprofile.View{Profile: profile, Preferences: &domain.Preferences{UserID: profile.UserID, MinAge: 18, MaxAge: 120, Sex: domain.SearchSexAny},
		Photos: []media.Photo{{ID: "22222222-2222-4222-8222-222222222222", URL: "https://photos.example.test/photo", Position: 1}}}
	for _, test := range []struct {
		method, path, body string
		service            *serviceStub
	}{
		{"GET", "/api/v1/profile", "", &serviceStub{view: view}},
		{"GET", "/api/v1/profile", "", &serviceStub{view: appprofile.View{Profile: profile}}},
		{"GET", "/api/v1/profile", "", &serviceStub{getErr: appprofile.ErrProfileNotFound}},
		{"GET", "/api/v1/profile", "", &serviceStub{getErr: errors.New("internal")}},
		{"PUT", "/api/v1/profile", `{"name":"Анна","age":25,"sex":"female","city":"Москва"}`, &serviceStub{profile: profile}},
		{"PUT", "/api/v1/profile", `{}`, &serviceStub{err: &appprofile.ValidationError{Field: "city", Message: "is required"}}},
		{"PUT", "/api/v1/profile", `{}`, &serviceStub{err: appprofile.ErrUserNotFound}},
		{"PUT", "/api/v1/profile", `{}`, &serviceStub{err: errors.New("internal")}},
		{"PUT", "/api/v1/profile", `{"age":"wrong"}`, &serviceStub{}},
		{"PUT", "/api/v1/preferences", `{"min_age":18,"max_age":120,"sex":"any"}`, &serviceStub{}},
		{"PUT", "/api/v1/preferences", `{}`, &serviceStub{err: appprofile.ErrProfileRequired}},
		{"PUT", "/api/v1/preferences", `{}`, &serviceStub{err: &appprofile.ValidationError{Field: "min_age", Message: "out of range"}}},
		{"PUT", "/api/v1/preferences", `{}`, &serviceStub{err: errors.New("internal")}},
		{"PATCH", "/api/v1/profile/sleep", `{"is_sleeping":true}`, &serviceStub{}},
		{"PATCH", "/api/v1/profile/sleep", `{}`, &serviceStub{}},
		{"PATCH", "/api/v1/profile/sleep", `{"is_sleeping":false}`, &serviceStub{err: appprofile.ErrProfileRequired}},
		{"PATCH", "/api/v1/profile/sleep", `{"is_sleeping":false}`, &serviceStub{err: errors.New("internal")}},
	} {
		response := serveAt(test.service, test.method, test.path, test.body)
		spec.CheckResponse(t, test.method, test.path, response)
	}
}
