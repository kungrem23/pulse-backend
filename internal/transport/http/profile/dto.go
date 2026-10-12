package profile

import (
	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

type updateRequest struct {
	Name string `json:"name"`
	Bio  string `json:"bio"`
	Age  int    `json:"age"`
	Sex  string `json:"sex"`
	City string `json:"city"`
}

type updatePreferencesRequest struct {
	MinAge int16  `json:"min_age"`
	MaxAge int16  `json:"max_age"`
	Sex    string `json:"sex"`
}

type setSleepRequest struct {
	IsSleeping bool `json:"is_sleeping"`
}

type profileResponse struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Bio        string `json:"bio"`
	Age        int    `json:"age"`
	Sex        string `json:"sex"`
	City       string `json:"city"`
	IsSleeping bool   `json:"is_sleeping"`
}

type preferencesResponse struct {
	UserID string `json:"user_id"`
	MinAge int16  `json:"min_age"`
	MaxAge int16  `json:"max_age"`
	Sex    string `json:"sex"`
}

type photoResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Position int16  `json:"position"`
}

type viewResponse struct {
	Profile     profileResponse      `json:"profile"`
	Preferences *preferencesResponse `json:"preferences"`
	Photos      []photoResponse      `json:"photos"`
}

func newProfileResponse(profile domain.Profile) profileResponse {
	return profileResponse{
		UserID:     profile.UserID,
		Name:       profile.Name,
		Bio:        profile.Bio,
		Age:        profile.Age,
		Sex:        string(profile.Sex),
		City:       profile.City,
		IsSleeping: profile.IsSleeping,
	}
}

func newViewResponse(view appprofile.View) viewResponse {
	response := viewResponse{
		Profile: newProfileResponse(view.Profile),
		Photos:  []photoResponse{},
	}
	if view.Preferences != nil {
		response.Preferences = &preferencesResponse{
			UserID: view.Preferences.UserID,
			MinAge: view.Preferences.MinAge,
			MaxAge: view.Preferences.MaxAge,
			Sex:    string(view.Preferences.Sex),
		}
	}
	return response
}
