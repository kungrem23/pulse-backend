package swipetest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type memoryRepository struct {
	mu      sync.Mutex
	users   map[string]UserSpec
	swipes  map[[2]string]bool
	matches map[[2]string]domain.Match
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		users:   map[string]UserSpec{},
		swipes:  map[[2]string]bool{},
		matches: map[[2]string]domain.Match{},
	}
}

func state(spec UserSpec) domain.SearchState {
	photos := 1
	if spec.NoPhotos {
		photos = 0
	}
	return domain.SearchState{
		IsBlocked:      spec.Blocked,
		HasProfile:     !spec.NoProfile,
		HasPreferences: !spec.NoPreferences,
		PhotoCount:     photos,
		IsSleeping:     spec.Sleeping,
	}
}

func wants(searcher, candidate UserSpec) bool {
	sexOK := searcher.SearchSex == domain.SearchSexAny || string(searcher.SearchSex) == string(candidate.Sex)
	return sexOK && candidate.Age >= searcher.MinAge && candidate.Age <= searcher.MaxAge
}

func ordered(a, b string) [2]string {
	if b < a {
		a, b = b, a
	}
	return [2]string{a, b}
}

func (r *memoryRepository) Save(_ context.Context, from, to string, isLike bool) (*domain.Match, error) {
	if from == to {
		return nil, domain.ErrSelfSwipe
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	caller, ok := r.users[from]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if err := state(caller).CheckEligible(); err != nil {
		return nil, err
	}
	target, ok := r.users[to]
	if !ok || state(target).CheckEligible() != nil || !wants(caller, target) || !wants(target, caller) {
		return nil, domain.ErrCandidateUnavailable
	}
	pair := ordered(from, to)
	if _, exists := r.swipes[[2]string{from, to}]; exists {
		return nil, domain.ErrConflict
	}
	if _, exists := r.matches[pair]; exists {
		return nil, domain.ErrConflict
	}
	r.swipes[[2]string{from, to}] = isLike
	if !isLike || !r.swipes[[2]string{to, from}] {
		return nil, nil
	}
	match := domain.Match{
		ID: uuid.NewString(), Person1ID: pair[0], Person2ID: pair[1], Status: "active", CreatedAt: time.Now(),
	}
	r.matches[pair] = match
	return &match, nil
}

func TestMemoryRepositorySatisfiesContract(t *testing.T) {
	Run(t, func(t *testing.T) Harness {
		repository := newMemoryRepository()
		return Harness{
			Repository: repository,
			CreateUser: func(_ *testing.T, spec UserSpec) string {
				id := uuid.NewString()
				repository.mu.Lock()
				defer repository.mu.Unlock()
				repository.users[id] = spec
				return id
			},
			Swipe: func(_ *testing.T, from, to string) (bool, bool) {
				repository.mu.Lock()
				defer repository.mu.Unlock()
				isLike, found := repository.swipes[[2]string{from, to}]
				return isLike, found
			},
			Matches: func(_ *testing.T, a, b string) []domain.Match {
				repository.mu.Lock()
				defer repository.mu.Unlock()
				if match, found := repository.matches[ordered(a, b)]; found {
					return []domain.Match{match}
				}
				return nil
			},
		}
	})
}
