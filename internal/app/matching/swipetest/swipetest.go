// Package swipetest is the behavioural contract of domain.SwipeRepository.
// A storage adapter passes it by calling Run from its own test with a Harness
// that seeds users in that storage, for example in an isolated PostgreSQL.
package swipetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type UserSpec struct {
	Age       int
	Sex       domain.Sex
	MinAge    int
	MaxAge    int
	SearchSex domain.SearchSex

	NoProfile     bool
	NoPreferences bool
	NoPhotos      bool
	Sleeping      bool
	Blocked       bool
}

type Harness struct {
	Repository domain.SwipeRepository
	CreateUser func(t *testing.T, spec UserSpec) string
	Swipe      func(t *testing.T, from, to string) (isLike, found bool)
	Matches    func(t *testing.T, a, b string) []domain.Match
}

const ConcurrentRounds = 25

func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	h := newHarness(t)
	ctx := context.Background()
	user := func(t *testing.T, sex domain.Sex, searchSex domain.SearchSex) string {
		return h.CreateUser(t, UserSpec{Age: 25, Sex: sex, MinAge: 18, MaxAge: 120, SearchSex: searchSex})
	}
	pair := func(t *testing.T) (string, string) {
		return user(t, domain.SexMale, domain.SearchSexFemale), user(t, domain.SexFemale, domain.SearchSexMale)
	}

	t.Run("like without reciprocal like", func(t *testing.T) {
		a, b := pair(t)
		match, err := h.Repository.Save(ctx, a, b, true)
		if err != nil || match != nil {
			t.Fatalf("expected no match, got %+v, %v", match, err)
		}
		if isLike, found := h.Swipe(t, a, b); !found || !isLike {
			t.Fatalf("like must be stored, found=%v isLike=%v", found, isLike)
		}
		if matches := h.Matches(t, a, b); len(matches) != 0 {
			t.Fatalf("unexpected matches %+v", matches)
		}
	})

	t.Run("mutual like creates one ordered match", func(t *testing.T) {
		a, b := pair(t)
		mustSave(t, h, a, b, true)
		match, err := h.Repository.Save(ctx, b, a, true)
		if err != nil || match == nil {
			t.Fatalf("expected a match, got %+v, %v", match, err)
		}
		checkMatch(t, *match, a, b)
		stored := h.Matches(t, a, b)
		if len(stored) != 1 || stored[0].ID != match.ID {
			t.Fatalf("expected the returned match to be the only stored one, got %+v", stored)
		}
	})

	t.Run("dislike never creates a match", func(t *testing.T) {
		a, b := pair(t)
		mustSave(t, h, a, b, false)
		if match, err := h.Repository.Save(ctx, b, a, true); err != nil || match != nil {
			t.Fatalf("like after the other side's dislike: %+v, %v", match, err)
		}
		c, d := pair(t)
		mustSave(t, h, c, d, true)
		if match, err := h.Repository.Save(ctx, d, c, false); err != nil || match != nil {
			t.Fatalf("dislike after the other side's like: %+v, %v", match, err)
		}
		if len(h.Matches(t, a, b)) != 0 || len(h.Matches(t, c, d)) != 0 {
			t.Fatal("dislikes must not create matches")
		}
	})

	t.Run("repeated swipe conflicts without rewriting history", func(t *testing.T) {
		a, b := pair(t)
		mustSave(t, h, a, b, true)
		for _, isLike := range []bool{true, false} {
			if _, err := h.Repository.Save(ctx, a, b, isLike); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("repeated swipe is_like=%v: expected ErrConflict, got %v", isLike, err)
			}
		}
		if isLike, found := h.Swipe(t, a, b); !found || !isLike {
			t.Fatal("the original like must be kept")
		}
	})

	t.Run("swipes after a match conflict and keep the match", func(t *testing.T) {
		a, b := pair(t)
		mustSave(t, h, a, b, true)
		match := mustSave(t, h, b, a, true)
		for _, from := range []string{a, b} {
			to := b
			if from == b {
				to = a
			}
			if _, err := h.Repository.Save(ctx, from, to, false); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("expected ErrConflict, got %v", err)
			}
		}
		stored := h.Matches(t, a, b)
		if len(stored) != 1 || match == nil || stored[0].ID != match.ID || stored[0].Status != "active" {
			t.Fatalf("the match must stay unchanged, got %+v", stored)
		}
	})

	t.Run("self swipe", func(t *testing.T) {
		a, _ := pair(t)
		if _, err := h.Repository.Save(ctx, a, a, true); !errors.Is(err, domain.ErrSelfSwipe) {
			t.Fatalf("expected ErrSelfSwipe, got %v", err)
		}
		if _, found := h.Swipe(t, a, a); found {
			t.Fatal("self swipe must not be stored")
		}
	})

	t.Run("missing caller", func(t *testing.T) {
		_, b := pair(t)
		if _, err := h.Repository.Save(ctx, uuid.NewString(), b, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ineligible caller", func(t *testing.T) {
		for _, test := range []struct {
			name string
			spec UserSpec
			want error
		}{
			{"blocked", UserSpec{Blocked: true}, domain.ErrBlocked},
			{"no profile", UserSpec{NoProfile: true}, domain.ErrProfileIncomplete},
			{"no preferences", UserSpec{NoPreferences: true}, domain.ErrProfileIncomplete},
			{"no photos", UserSpec{NoPhotos: true}, domain.ErrNoPhotos},
			{"sleeping", UserSpec{Sleeping: true}, domain.ErrSleeping},
		} {
			t.Run(test.name, func(t *testing.T) {
				spec := test.spec
				spec.Age, spec.Sex, spec.MinAge, spec.MaxAge, spec.SearchSex = 25, domain.SexMale, 18, 120, domain.SearchSexFemale
				caller := h.CreateUser(t, spec)
				target := user(t, domain.SexFemale, domain.SearchSexMale)
				if _, err := h.Repository.Save(ctx, caller, target, true); !errors.Is(err, test.want) {
					t.Fatalf("expected %v, got %v", test.want, err)
				}
				if _, found := h.Swipe(t, caller, target); found {
					t.Fatal("rejected swipe must not be stored")
				}
			})
		}
	})

	t.Run("unavailable candidate", func(t *testing.T) {
		male := UserSpec{Age: 25, Sex: domain.SexMale, MinAge: 18, MaxAge: 120, SearchSex: domain.SearchSexFemale}
		female := UserSpec{Age: 25, Sex: domain.SexFemale, MinAge: 18, MaxAge: 120, SearchSex: domain.SearchSexMale}
		with := func(spec UserSpec, change func(*UserSpec)) UserSpec {
			change(&spec)
			return spec
		}
		for _, test := range []struct {
			name           string
			caller, target UserSpec
		}{
			{"blocked", male, with(female, func(s *UserSpec) { s.Blocked = true })},
			{"no profile", male, with(female, func(s *UserSpec) { s.NoProfile = true })},
			{"no preferences", male, with(female, func(s *UserSpec) { s.NoPreferences = true })},
			{"no photos", male, with(female, func(s *UserSpec) { s.NoPhotos = true })},
			{"sleeping", male, with(female, func(s *UserSpec) { s.Sleeping = true })},
			{"target too old for caller", with(male, func(s *UserSpec) { s.MaxAge = 24 }), female},
			{"target too young for caller", with(male, func(s *UserSpec) { s.MinAge = 26 }), female},
			{"caller too old for target", male, with(female, func(s *UserSpec) { s.MaxAge = 24 })},
			{"caller too young for target", male, with(female, func(s *UserSpec) { s.MinAge = 26 })},
			{"caller does not want target sex", with(male, func(s *UserSpec) { s.SearchSex = domain.SearchSexMale }), female},
			{"target does not want caller sex", male, with(female, func(s *UserSpec) { s.SearchSex = domain.SearchSexFemale })},
		} {
			t.Run(test.name, func(t *testing.T) {
				caller, target := h.CreateUser(t, test.caller), h.CreateUser(t, test.target)
				if _, err := h.Repository.Save(ctx, caller, target, true); !errors.Is(err, domain.ErrCandidateUnavailable) {
					t.Fatalf("expected ErrCandidateUnavailable, got %v", err)
				}
				if _, found := h.Swipe(t, caller, target); found {
					t.Fatal("rejected swipe must not be stored")
				}
			})
		}
		t.Run("missing target", func(t *testing.T) {
			caller := h.CreateUser(t, male)
			if _, err := h.Repository.Save(ctx, caller, uuid.NewString(), true); !errors.Is(err, domain.ErrCandidateUnavailable) {
				t.Fatalf("expected ErrCandidateUnavailable, got %v", err)
			}
		})
	})

	t.Run("age bounds and any are inclusive", func(t *testing.T) {
		a := h.CreateUser(t, UserSpec{Age: 30, Sex: domain.SexMale, MinAge: 20, MaxAge: 20, SearchSex: domain.SearchSexAny})
		b := h.CreateUser(t, UserSpec{Age: 20, Sex: domain.SexMale, MinAge: 30, MaxAge: 30, SearchSex: domain.SearchSexAny})
		mustSave(t, h, a, b, true)
		if match := mustSave(t, h, b, a, true); match == nil {
			t.Fatal("exact age bounds and sex any must match")
		}
	})

	t.Run("concurrent mutual likes create exactly one match", func(t *testing.T) {
		for round := 0; round < ConcurrentRounds; round++ {
			a, b := pair(t)
			results := saveConcurrently(h, [][2]string{{a, b}, {b, a}}, true)
			created := 0
			for _, result := range results {
				if result.err != nil {
					t.Fatalf("round %d: unexpected error %v", round, result.err)
				}
				if result.match != nil {
					created++
					checkMatch(t, *result.match, a, b)
				}
			}
			if created != 1 {
				t.Fatalf("round %d: expected exactly one call to return the match, got %d", round, created)
			}
			if stored := h.Matches(t, a, b); len(stored) != 1 {
				t.Fatalf("round %d: expected one stored match, got %+v", round, stored)
			}
		}
	})

	t.Run("concurrent identical swipes store one", func(t *testing.T) {
		a, b := pair(t)
		calls := make([][2]string, 8)
		for i := range calls {
			calls[i] = [2]string{a, b}
		}
		succeeded := 0
		for _, result := range saveConcurrently(h, calls, true) {
			switch {
			case result.err == nil:
				succeeded++
			case !errors.Is(result.err, domain.ErrConflict):
				t.Fatalf("expected ErrConflict for duplicates, got %v", result.err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("expected exactly one stored swipe, got %d successes", succeeded)
		}
	})
}

type saveResult struct {
	match *domain.Match
	err   error
}

func saveConcurrently(h Harness, calls [][2]string, isLike bool) []saveResult {
	results := make([]saveResult, len(calls))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			match, err := h.Repository.Save(context.Background(), call[0], call[1], isLike)
			results[i] = saveResult{match, err}
		}()
	}
	close(start)
	wg.Wait()
	return results
}

func mustSave(t *testing.T, h Harness, from, to string, isLike bool) *domain.Match {
	t.Helper()
	match, err := h.Repository.Save(context.Background(), from, to, isLike)
	if err != nil {
		t.Fatalf("save %s -> %s: %v", from, to, err)
	}
	return match
}

func checkMatch(t *testing.T, match domain.Match, a, b string) {
	t.Helper()
	first, second := a, b
	if second < first {
		first, second = second, first
	}
	if _, err := uuid.Parse(match.ID); err != nil {
		t.Errorf("match ID must be a UUID: %q", match.ID)
	}
	if match.Person1ID != first || match.Person2ID != second {
		t.Errorf("pair must be ordered (%s, %s), got (%s, %s)", first, second, match.Person1ID, match.Person2ID)
	}
	if match.Status != "active" || match.ConversationID != nil || match.CreatedAt.IsZero() {
		t.Errorf("unexpected new match %s", fmt.Sprintf("%+v", match))
	}
}
