// Package profile describes profile and search-preference use cases.
package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	maxNameLength = 100
	maxCityLength = 100
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrUserNotFound    = errors.New("user not found")
)

// ValidationError reports the first invalid field of a request.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

type View struct {
	Profile domain.Profile
	// Preferences is nil until the user sets them; no defaults are invented.
	Preferences *domain.Preferences
	Photos      []domain.Photo
}

type UpdateCommand struct {
	UserID string
	Name   string
	Bio    string
	Age    int
	Sex    string
	City   string
}

type Service interface {
	Get(ctx context.Context, userID string) (View, error)
	Update(context.Context, UpdateCommand) (domain.Profile, error)
}

type service struct {
	profiles domain.ProfileRepository
}

func NewService(profiles domain.ProfileRepository) Service {
	return &service{profiles: profiles}
}

func (s *service) Get(ctx context.Context, userID string) (View, error) {
	profile, err := s.profiles.ByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return View{}, ErrProfileNotFound
		}
		return View{}, fmt.Errorf("get profile: %w", err)
	}

	// Photos are not listed until photo storage is wired.
	view := View{Profile: profile, Photos: []domain.Photo{}}
	preferences, err := s.profiles.Preferences(ctx, userID)
	switch {
	case err == nil:
		view.Preferences = &preferences
	case !errors.Is(err, domain.ErrNotFound):
		return View{}, fmt.Errorf("get preferences: %w", err)
	}
	return view, nil
}

func (s *service) Update(ctx context.Context, command UpdateCommand) (domain.Profile, error) {
	profile, err := validateUpdate(command)
	if err != nil {
		return domain.Profile{}, err
	}
	saved, err := s.profiles.Save(ctx, profile)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Profile{}, ErrUserNotFound
		}
		return domain.Profile{}, fmt.Errorf("save profile: %w", err)
	}
	return saved, nil
}

func validateUpdate(command UpdateCommand) (domain.Profile, error) {
	name := strings.TrimSpace(command.Name)
	bio := strings.TrimSpace(command.Bio)
	city := strings.TrimSpace(command.City)
	sex := domain.Sex(command.Sex)

	switch {
	case name == "" || utf8.RuneCountInString(name) > maxNameLength:
		return domain.Profile{}, &ValidationError{"name", fmt.Sprintf("must contain 1 to %d characters", maxNameLength)}
	case utf8.RuneCountInString(bio) > domain.MaxBioLength:
		return domain.Profile{}, &ValidationError{"bio", fmt.Sprintf("must contain at most %d characters", domain.MaxBioLength)}
	case command.Age < domain.MinAge || command.Age > domain.MaxAge:
		return domain.Profile{}, &ValidationError{"age", fmt.Sprintf("must be between %d and %d", domain.MinAge, domain.MaxAge)}
	case sex != domain.SexMale && sex != domain.SexFemale:
		return domain.Profile{}, &ValidationError{"sex", "must be male or female"}
	case city == "" || utf8.RuneCountInString(city) > maxCityLength:
		return domain.Profile{}, &ValidationError{"city", fmt.Sprintf("must contain 1 to %d characters", maxCityLength)}
	}
	return domain.Profile{
		UserID: command.UserID,
		Name:   name,
		Bio:    bio,
		Age:    command.Age,
		Sex:    sex,
		City:   city,
	}, nil
}
