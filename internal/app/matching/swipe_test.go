package matching

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	callerID = "11111111-1111-4111-8111-111111111111"
	targetID = "22222222-2222-4222-8222-222222222222"
)

var eligibleState = domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: 1}

type stateRepositoryStub struct {
	state  domain.SearchState
	err    error
	userID string
}

func (s *stateRepositoryStub) StateByUserID(_ context.Context, userID string) (domain.SearchState, error) {
	s.userID = userID
	return s.state, s.err
}

type swipeRepositoryStub struct {
	match *domain.Match
	err   error
	calls int

	from, to string
	isLike   bool
}

func (s *swipeRepositoryStub) Save(_ context.Context, from, to string, isLike bool) (*domain.Match, error) {
	s.calls++
	s.from, s.to, s.isLike = from, to, isLike
	return s.match, s.err
}

func newTestService(states *stateRepositoryStub, swipes *swipeRepositoryStub) Service {
	return NewService(swipes, nil, states, nil)
}

func TestSwipeSavesLikeWithoutMatch(t *testing.T) {
	states := &stateRepositoryStub{state: eligibleState}
	swipes := &swipeRepositoryStub{}

	match, err := newTestService(states, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: targetID, IsLike: true,
	})
	if err != nil || match != nil {
		t.Fatalf("expected no match and no error, got %+v, %v", match, err)
	}
	if states.userID != callerID {
		t.Fatalf("state must be checked for the caller, got %q", states.userID)
	}
	if swipes.calls != 1 || swipes.from != callerID || swipes.to != targetID || !swipes.isLike {
		t.Fatalf("unexpected save: %+v", swipes)
	}
}

func TestSwipeReturnsCreatedMatch(t *testing.T) {
	created := &domain.Match{ID: "33333333-3333-4333-8333-333333333333", Status: "active"}
	swipes := &swipeRepositoryStub{match: created}

	match, err := newTestService(&stateRepositoryStub{state: eligibleState}, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: targetID, IsLike: true,
	})
	if err != nil || match != created {
		t.Fatalf("expected the created match, got %+v, %v", match, err)
	}
}

func TestSwipePassesDislike(t *testing.T) {
	swipes := &swipeRepositoryStub{}
	_, err := newTestService(&stateRepositoryStub{state: eligibleState}, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: targetID, IsLike: false,
	})
	if err != nil || swipes.calls != 1 || swipes.isLike {
		t.Fatalf("dislike must be saved as is_like=false: %+v, %v", swipes, err)
	}
}

func TestSwipeRejectsMatchForDislike(t *testing.T) {
	swipes := &swipeRepositoryStub{match: &domain.Match{ID: "33333333-3333-4333-8333-333333333333"}}
	match, err := newTestService(&stateRepositoryStub{state: eligibleState}, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: targetID, IsLike: false,
	})
	if err == nil || match != nil {
		t.Fatalf("a match for a dislike must be an internal error, got %+v, %v", match, err)
	}
}

func TestSwipeNormalizesUUIDCase(t *testing.T) {
	swipes := &swipeRepositoryStub{}
	_, err := newTestService(&stateRepositoryStub{state: eligibleState}, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: strings.ToUpper(targetID), IsLike: true,
	})
	if err != nil || swipes.to != targetID {
		t.Fatalf("target must be passed in lowercase, got %q, %v", swipes.to, err)
	}
}

func TestSwipeRejectsBeforeRepository(t *testing.T) {
	for _, test := range []struct {
		name    string
		from    string
		to      string
		state   domain.SearchState
		stateEr error
		want    error
	}{
		{"empty target", callerID, "", eligibleState, nil, domain.ErrInvalidUserID},
		{"malformed target", callerID, "not-a-uuid", eligibleState, nil, domain.ErrInvalidUserID},
		{"compact target", callerID, strings.ReplaceAll(targetID, "-", ""), eligibleState, nil, domain.ErrInvalidUserID},
		{"urn target", callerID, "urn:uuid:" + targetID, eligibleState, nil, domain.ErrInvalidUserID},
		{"self", callerID, callerID, eligibleState, nil, domain.ErrSelfSwipe},
		{"self in other case", callerID, strings.ToUpper(callerID), eligibleState, nil, domain.ErrSelfSwipe},
		{"malformed caller", "user-from-token", targetID, eligibleState, nil, domain.ErrNotFound},
		{"missing caller", callerID, targetID, domain.SearchState{}, domain.ErrNotFound, domain.ErrNotFound},
		{"blocked caller", callerID, targetID, domain.SearchState{IsBlocked: true, HasProfile: true, HasPreferences: true, PhotoCount: 1}, nil, domain.ErrBlocked},
		{"no profile", callerID, targetID, domain.SearchState{HasPreferences: true, PhotoCount: 1}, nil, domain.ErrProfileIncomplete},
		{"no preferences", callerID, targetID, domain.SearchState{HasProfile: true, PhotoCount: 1}, nil, domain.ErrProfileIncomplete},
		{"no photos", callerID, targetID, domain.SearchState{HasProfile: true, HasPreferences: true}, nil, domain.ErrNoPhotos},
		{"sleeping caller", callerID, targetID, domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: 1, IsSleeping: true}, nil, domain.ErrSleeping},
	} {
		t.Run(test.name, func(t *testing.T) {
			swipes := &swipeRepositoryStub{}
			_, err := newTestService(&stateRepositoryStub{state: test.state, err: test.stateEr}, swipes).Swipe(
				context.Background(), SwipeCommand{FromUserID: test.from, ToUserID: test.to, IsLike: true},
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if swipes.calls != 0 {
				t.Fatal("repository must not be called")
			}
		})
	}
}

func TestSwipeKeepsRepositoryErrors(t *testing.T) {
	for _, want := range []error{
		domain.ErrConflict, domain.ErrCandidateUnavailable, domain.ErrNotFound, domain.ErrSleeping,
	} {
		swipes := &swipeRepositoryStub{err: want}
		_, err := newTestService(&stateRepositoryStub{state: eligibleState}, swipes).Swipe(context.Background(), SwipeCommand{
			FromUserID: callerID, ToUserID: targetID, IsLike: true,
		})
		if !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	}
}

func TestSwipeStateFailureIsNotADomainError(t *testing.T) {
	failure := errors.New("database is down")
	swipes := &swipeRepositoryStub{}
	_, err := newTestService(&stateRepositoryStub{err: failure}, swipes).Swipe(context.Background(), SwipeCommand{
		FromUserID: callerID, ToUserID: targetID, IsLike: true,
	})
	if !errors.Is(err, failure) || errors.Is(err, domain.ErrNotFound) || swipes.calls != 0 {
		t.Fatalf("unexpected result: %v, calls %d", err, swipes.calls)
	}
}
