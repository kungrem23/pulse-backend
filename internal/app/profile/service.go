// Package profile describes profile and search-preference use cases.
package profile

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type View struct {
	Profile     domain.Profile
	Preferences domain.Preferences
	Photos      []domain.Photo
}

type Service interface {
	Get(context.Context, string) (View, error)
	Update(context.Context, string, domain.Profile) error
	UpdatePreferences(context.Context, string, domain.Preferences) error
	SetSleep(context.Context, string, bool) error
}
