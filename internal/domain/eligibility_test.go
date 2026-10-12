package domain

import (
	"errors"
	"testing"
)

func TestSearchStateCheckEligible(t *testing.T) {
	ready := SearchState{HasProfile: true, HasPreferences: true, PhotoCount: 1}
	tests := []struct {
		name   string
		mutate func(*SearchState)
		want   error
	}{
		{"complete profile with one photo", func(*SearchState) {}, nil},
		{"five photos", func(s *SearchState) { s.PhotoCount = 5 }, nil},
		{"no profile", func(s *SearchState) { s.HasProfile = false }, ErrProfileIncomplete},
		{"no preferences", func(s *SearchState) { s.HasPreferences = false }, ErrProfileIncomplete},
		{"no photos", func(s *SearchState) { s.PhotoCount = 0 }, ErrNoPhotos},
		{"sleeping", func(s *SearchState) { s.IsSleeping = true }, ErrSleeping},
		{"blocked wins over everything", func(s *SearchState) { *s = SearchState{IsBlocked: true, IsSleeping: true} }, ErrBlocked},
		{"woken up but without photos", func(s *SearchState) { s.IsSleeping = false; s.PhotoCount = 0 }, ErrNoPhotos},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := ready
			test.mutate(&state)
			if err := state.CheckEligible(); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}
