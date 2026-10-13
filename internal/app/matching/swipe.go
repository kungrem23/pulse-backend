package matching

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

var errNotImplemented = errors.New("operation is not implemented")

type service struct {
	swipes  domain.SwipeRepository
	matches domain.MatchRepository
	states  domain.SearchStateRepository
	photos  domain.PhotoStorage
}

func NewService(
	swipes domain.SwipeRepository,
	matches domain.MatchRepository,
	states domain.SearchStateRepository,
	photos domain.PhotoStorage,
) Service {
	return &service{swipes: swipes, matches: matches, states: states, photos: photos}
}

// Swipe returns domain errors for HTTP to map with errors.Is:
// ErrInvalidUserID (target), ErrSelfSwipe, ErrNotFound (caller is gone),
// eligibility errors of the caller, ErrCandidateUnavailable and ErrConflict.
func (s *service) Swipe(ctx context.Context, command SwipeCommand) (*domain.Match, error) {
	// The caller ID comes from a verified token; a malformed one cannot
	// identify an existing user.
	from, ok := canonicalUUID(command.FromUserID)
	if !ok {
		return nil, fmt.Errorf("swipe: caller: %w", domain.ErrNotFound)
	}
	to, ok := canonicalUUID(command.ToUserID)
	if !ok {
		return nil, fmt.Errorf("swipe: target: %w", domain.ErrInvalidUserID)
	}
	if from == to {
		return nil, fmt.Errorf("swipe: %w", domain.ErrSelfSwipe)
	}

	// A cheap precheck with a clear error; the repository repeats all checks
	// for both users atomically with the write.
	state, err := s.states.StateByUserID(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("swipe: load caller state: %w", err)
	}
	if err := state.CheckEligible(); err != nil {
		return nil, fmt.Errorf("swipe: %w", err)
	}

	match, err := s.swipes.Save(ctx, from, to, command.IsLike)
	if err != nil {
		return nil, fmt.Errorf("swipe: save: %w", err)
	}
	if match != nil && !command.IsLike {
		return nil, errors.New("swipe: repository created a match for a dislike")
	}
	return match, nil
}

// Close is post-MVP.
func (s *service) Close(context.Context, string, string) error {
	return errNotImplemented
}

// canonicalUUID accepts only the standard hyphenated form, in any letter case.
func canonicalUUID(value string) (string, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != strings.ToLower(value) {
		return "", false
	}
	return parsed.String(), true
}
