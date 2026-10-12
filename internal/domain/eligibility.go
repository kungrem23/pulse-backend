package domain

import "errors"

var (
	ErrProfileIncomplete = errors.New("profile and preferences must be filled in")
	ErrNoPhotos          = errors.New("at least one photo is required")
	ErrSleeping          = errors.New("profile is in sleep mode")
	ErrBlocked           = errors.New("user is blocked")
)

// SearchState holds everything that decides whether a user takes part in
// discovery: sees the feed, swipes, and is shown to others.
type SearchState struct {
	IsBlocked      bool
	HasProfile     bool
	HasPreferences bool
	PhotoCount     int
	IsSleeping     bool
}

// CheckEligible returns nil when the user may take part in discovery, otherwise
// the first unmet rule. The same rules apply to both sides of a feed or swipe.
func (s SearchState) CheckEligible() error {
	switch {
	case s.IsBlocked:
		return ErrBlocked
	case !s.HasProfile || !s.HasPreferences:
		return ErrProfileIncomplete
	case s.PhotoCount < 1:
		return ErrNoPhotos
	case s.IsSleeping:
		return ErrSleeping
	}
	return nil
}
