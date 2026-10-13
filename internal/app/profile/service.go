// Package profile describes profile and search-preference use cases.
package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	maxNameLength = 100
	maxCityLength = 100
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrUserNotFound    = errors.New("user not found")
	// ErrProfileRequired means the operation needs an existing profile.
	ErrProfileRequired = errors.New("profile must be created first")
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
	Photos      []media.Photo
}

type UpdateCommand struct {
	UserID string
	Name   string
	Bio    string
	Age    int
	Sex    string
	City   string
}

type PreferencesCommand struct {
	UserID string
	MinAge int
	MaxAge int
	Sex    string
}

type Service interface {
	Get(ctx context.Context, userID string) (View, error)
	Update(context.Context, UpdateCommand) (domain.Profile, error)
	UpdatePreferences(context.Context, PreferencesCommand) (domain.Preferences, error)
	// SetSleep returns the stored sleep state.
	SetSleep(ctx context.Context, userID string, isSleeping bool) (bool, error)
}

type service struct {
	profiles domain.ProfileRepository
	photos   domain.PhotoRepository
	storage  domain.PhotoStorage
}

func NewService(profiles domain.ProfileRepository, photos domain.PhotoRepository, storage domain.PhotoStorage) Service {
	return &service{profiles: profiles, photos: photos, storage: storage}
}

func (s *service) Get(ctx context.Context, userID string) (View, error) {
	profile, err := s.profiles.ByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return View{}, ErrProfileNotFound
		}
		return View{}, fmt.Errorf("get profile: %w", err)
	}

	view := View{Profile: profile, Photos: []media.Photo{}}
	preferences, err := s.profiles.Preferences(ctx, userID)
	switch {
	case err == nil:
		view.Preferences = &preferences
	case !errors.Is(err, domain.ErrNotFound):
		return View{}, fmt.Errorf("get preferences: %w", err)
	}
	photos, err := s.photos.ByUserID(ctx, userID)
	if err != nil {
		return View{}, fmt.Errorf("get profile photos: %w", err)
	}
	for _, photo := range photos {
		url, err := s.storage.URL(ctx, photo.ObjectKey)
		if err != nil {
			return View{}, fmt.Errorf("get profile photo URL: %w", err)
		}
		view.Photos = append(view.Photos, media.Photo{ID: photo.ID, URL: url, Position: photo.Position})
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

func (s *service) UpdatePreferences(ctx context.Context, command PreferencesCommand) (domain.Preferences, error) {
	preferences, err := validatePreferences(command)
	if err != nil {
		return domain.Preferences{}, err
	}
	saved, err := s.profiles.SavePreferences(ctx, preferences)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Preferences{}, ErrProfileRequired
		}
		return domain.Preferences{}, fmt.Errorf("save preferences: %w", err)
	}
	return saved, nil
}

func (s *service) SetSleep(ctx context.Context, userID string, isSleeping bool) (bool, error) {
	stored, err := s.profiles.SetSleep(ctx, userID, isSleeping)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, ErrProfileRequired
		}
		return false, fmt.Errorf("set sleep mode: %w", err)
	}
	return stored, nil
}

func validatePreferences(command PreferencesCommand) (domain.Preferences, error) {
	sex := domain.SearchSex(command.Sex)
	ageRange := fmt.Sprintf("must be between %d and %d", domain.MinAge, domain.MaxAge)
	switch {
	case command.MinAge < domain.MinAge || command.MinAge > domain.MaxAge:
		return domain.Preferences{}, &ValidationError{"min_age", ageRange}
	case command.MaxAge < domain.MinAge || command.MaxAge > domain.MaxAge:
		return domain.Preferences{}, &ValidationError{"max_age", ageRange}
	case command.MinAge > command.MaxAge:
		return domain.Preferences{}, &ValidationError{"min_age", "must not exceed max_age"}
	case sex != domain.SearchSexMale && sex != domain.SearchSexFemale && sex != domain.SearchSexAny:
		return domain.Preferences{}, &ValidationError{"sex", "must be male, female or any"}
	}
	return domain.Preferences{
		UserID: command.UserID,
		MinAge: int16(command.MinAge),
		MaxAge: int16(command.MaxAge),
		Sex:    sex,
	}, nil
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
