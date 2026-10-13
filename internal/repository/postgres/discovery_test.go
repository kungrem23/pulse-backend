package postgres_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

// TODO(db): test-fixture SQL is also left as placeholders at the owner's request.
// These run ONLY on pgtest's throwaway database, never on the user's database.
const (
	// Params: $1 user UUID. Set users.is_blocked=true for one test user.
	discoveryTestBlockUserQuery = ""
	// Params: $1 from UUID, $2 to UUID, $3 is_like. Insert a swipe.
	discoveryTestSwipeQuery = ""
	// Params: $1 ordered person1 UUID, $2 person2 UUID, $3 active|closed.
	// Insert match; closed_at must be set for closed, NULL for active.
	discoveryTestMatchQuery = ""
	// Params: $1 timestamp, $2 UUID array. Set users.created_at for only these IDs
	// so tests exercise timestamp ties and UUID ordering, not time-based luck.
	discoveryTestCreatedAtQuery = ""
)

type discoveryFixture struct {
	pool     *pgxpool.Pool
	users    *postgres.UserRepository
	profiles *postgres.ProfileRepository
	photos   *postgres.PhotoRepository
	feed     *postgres.DiscoveryRepository
	states   *postgres.SearchStateRepository
}

func newDiscoveryFixture(t *testing.T) *discoveryFixture {
	t.Helper()
	pool := pgtest.NewDatabase(t)
	return &discoveryFixture{pool: pool, users: postgres.NewUserRepository(pool), profiles: postgres.NewProfileRepository(pool),
		photos: postgres.NewPhotoRepository(pool), feed: postgres.NewDiscoveryRepository(pool), states: postgres.NewSearchStateRepository(pool)}
}

func (f *discoveryFixture) user(t *testing.T, age int16, sex domain.Sex, hasProfile, hasPreferences, hasPhoto bool) domain.User {
	t.Helper()
	ctx := context.Background()
	user, err := f.users.Create(ctx, domain.User{Email: uuid.NewString() + "@example.test", PasswordHash: "hash", Age: int(age)})
	if err != nil {
		t.Fatal(err)
	}
	if hasProfile {
		if _, err := f.profiles.Save(ctx, domain.Profile{UserID: user.ID, Name: "Анна", Age: int(age), Sex: sex, City: "Казань"}); err != nil {
			t.Fatal(err)
		}
	}
	if hasPreferences {
		f.preferences(t, user.ID, 18, 120, domain.SearchSexAny)
	}
	if hasPhoto {
		id := uuid.NewString()
		if err := f.photos.Create(ctx, domain.Photo{ID: id, UserID: user.ID, ObjectKey: "test/" + id, Position: 1}); err != nil {
			t.Fatal(err)
		}
	}
	return user
}

func (f *discoveryFixture) preferences(t *testing.T, owner string, min, max int16, sex domain.SearchSex) {
	t.Helper()
	if _, err := f.profiles.SavePreferences(context.Background(), domain.Preferences{UserID: owner, MinAge: min, MaxAge: max, Sex: sex}); err != nil {
		t.Fatal(err)
	}
}

func (f *discoveryFixture) fixtureSQL(t *testing.T, query string, args ...any) {
	t.Helper()
	if query == "" {
		t.Fatal("T09 test fixture SQL is not written yet; fill the documented placeholder")
	}
	if _, err := f.pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryMutualFiltersWithPostgres(t *testing.T) {
	for _, test := range []struct {
		name                                             string
		callerAge, candidateAge                          int16
		callerSex, candidateSex                          domain.Sex
		callerMin, callerMax, candidateMin, candidateMax int16
		callerWant, candidateWant                        domain.SearchSex
		want                                             bool
	}{
		{"all four filters", 30, 28, domain.SexMale, domain.SexFemale, 25, 35, 20, 40, domain.SearchSexFemale, domain.SearchSexMale, true},
		{"caller rejects age", 30, 28, domain.SexMale, domain.SexFemale, 29, 35, 20, 40, domain.SearchSexFemale, domain.SearchSexMale, false},
		{"caller rejects sex", 30, 28, domain.SexMale, domain.SexFemale, 25, 35, 20, 40, domain.SearchSexMale, domain.SearchSexMale, false},
		{"candidate rejects age", 30, 28, domain.SexMale, domain.SexFemale, 25, 35, 31, 40, domain.SearchSexFemale, domain.SearchSexMale, false},
		{"candidate rejects sex", 30, 28, domain.SexMale, domain.SexFemale, 25, 35, 20, 40, domain.SearchSexFemale, domain.SearchSexFemale, false},
		{"caller any", 30, 28, domain.SexMale, domain.SexMale, 25, 35, 20, 40, domain.SearchSexAny, domain.SearchSexMale, true},
		{"candidate any", 30, 28, domain.SexFemale, domain.SexFemale, 25, 35, 20, 40, domain.SearchSexFemale, domain.SearchSexAny, true},
		{"both any and age edges", 18, 120, domain.SexMale, domain.SexFemale, 120, 120, 18, 18, domain.SearchSexAny, domain.SearchSexAny, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDiscoveryFixture(t)
			caller := f.user(t, test.callerAge, test.callerSex, true, true, true)
			candidate := f.user(t, test.candidateAge, test.candidateSex, true, true, true)
			// Different cities are still compatible; city is not a filter.
			if _, err := f.profiles.Save(context.Background(), domain.Profile{UserID: caller.ID, Name: "Борис", Age: int(test.callerAge), Sex: test.callerSex, City: "Москва"}); err != nil {
				t.Fatal(err)
			}
			f.preferences(t, caller.ID, test.callerMin, test.callerMax, test.callerWant)
			f.preferences(t, candidate.ID, test.candidateMin, test.candidateMax, test.candidateWant)
			wantPhotos := 1
			if test.name == "all four filters" {
				for _, position := range []int16{5, 3} {
					id := uuid.NewString()
					if err := f.photos.Create(context.Background(), domain.Photo{ID: id, UserID: candidate.ID, ObjectKey: "test/" + id, Position: position}); err != nil {
						t.Fatal(err)
					}
				}
				wantPhotos = 3
			}
			page, err := f.feed.Feed(context.Background(), caller.ID, domain.PageQuery{Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if test.want {
				wantCount = 1
			}
			if len(page.Items) != wantCount || page.HasMore {
				t.Fatalf("mutual filters: %+v", page)
			}
			if test.want && (page.Items[0].Profile.UserID != candidate.ID || len(page.Items[0].Photos) != wantPhotos) {
				t.Fatalf("wrong candidate/photo projection: %+v", page.Items)
			}
			if test.want && wantPhotos == 3 {
				for i, position := range []int16{1, 3, 5} {
					if page.Items[0].Photos[i].Position != position {
						t.Fatalf("bulk photos must be in position order: %+v", page.Items[0].Photos)
					}
				}
			}
		})
	}
}

func TestDiscoveryExclusionsWithPostgres(t *testing.T) {
	for _, exclusion := range []string{"self", "sleeping", "blocked", "no profile", "no preferences", "no photos", "outgoing like", "outgoing pass", "active match", "closed match", "incoming like only"} {
		t.Run(exclusion, func(t *testing.T) {
			f := newDiscoveryFixture(t)
			caller := f.user(t, 30, domain.SexMale, true, true, true)
			candidate := caller
			if exclusion != "self" {
				candidate = f.user(t, 28, domain.SexFemale, exclusion != "no profile", exclusion != "no profile" && exclusion != "no preferences", exclusion != "no profile" && exclusion != "no photos")
			}
			switch exclusion {
			case "sleeping":
				if _, err := f.profiles.SetSleep(context.Background(), candidate.ID, true); err != nil {
					t.Fatal(err)
				}
			case "blocked":
				f.fixtureSQL(t, discoveryTestBlockUserQuery, candidate.ID)
			case "outgoing like", "outgoing pass":
				f.fixtureSQL(t, discoveryTestSwipeQuery, caller.ID, candidate.ID, exclusion == "outgoing like")
			case "incoming like only":
				f.fixtureSQL(t, discoveryTestSwipeQuery, candidate.ID, caller.ID, true)
			case "active match", "closed match":
				left, right := caller.ID, candidate.ID
				if left > right {
					left, right = right, left
				}
				status := "active"
				if exclusion == "closed match" {
					status = "closed"
				}
				f.fixtureSQL(t, discoveryTestMatchQuery, left, right, status)
			}
			page, err := f.feed.Feed(context.Background(), caller.ID, domain.PageQuery{Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if exclusion == "incoming like only" {
				want = 1
			}
			if len(page.Items) != want || page.Items == nil {
				t.Fatalf("%s exclusion: %+v", exclusion, page)
			}
		})
	}
}

func TestDiscoveryPaginationWithPostgres(t *testing.T) {
	f := newDiscoveryFixture(t)
	caller := f.user(t, 30, domain.SexMale, true, true, true)
	ids := make([]string, 0, 7)
	for range 7 {
		ids = append(ids, f.user(t, 25, domain.SexFemale, true, true, true).ID)
	}
	sameTime := time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC)
	f.fixtureSQL(t, discoveryTestCreatedAtQuery, sameTime, ids)
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	query := domain.PageQuery{Limit: 2}
	var got []string
	for {
		page, err := f.feed.Feed(context.Background(), caller.ID, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 2 || len(got) > 7 {
			t.Fatal("pagination exceeded limit or repeated items")
		}
		for _, item := range page.Items {
			if !item.CreatedAt.Equal(sameTime) {
				t.Fatal("timestamp fixture did not set a shared pagination timestamp")
			}
			got = append(got, item.Profile.UserID)
		}
		if !page.HasMore {
			break
		}
		last := page.Items[len(page.Items)-1]
		query.After = &domain.PagePosition{CreatedAt: last.CreatedAt, ID: last.Profile.UserID}
	}
	if len(got) != len(ids) {
		t.Fatalf("expected all seven candidates, got %d", len(got))
	}
	for i, id := range ids {
		if got[i] != id {
			t.Fatalf("timestamp tie must be ordered by UUID DESC: %+v", got)
		}
	}
	query.After = &domain.PagePosition{CreatedAt: sameTime, ID: ids[len(ids)-1]}
	page, err := f.feed.Feed(context.Background(), caller.ID, query)
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.HasMore {
		t.Fatalf("exhausted page: %+v, %v", page, err)
	}
}

func TestSearchStateAndCallerRecheckWithPostgres(t *testing.T) {
	f := newDiscoveryFixture(t)
	ctx := context.Background()
	user := f.user(t, 30, domain.SexMale, false, false, false)
	state, err := f.states.StateByUserID(ctx, user.ID)
	if err != nil || state.HasProfile || state.HasPreferences || state.PhotoCount != 0 {
		t.Fatalf("unstarted onboarding state: %+v, %v", state, err)
	}
	if _, err := f.feed.Feed(ctx, user.ID, domain.PageQuery{Limit: 20}); !errors.Is(err, domain.ErrProfileIncomplete) {
		t.Fatalf("incomplete caller: %v", err)
	}
	caller := f.user(t, 30, domain.SexMale, true, true, true)
	state, err = f.states.StateByUserID(ctx, caller.ID)
	if err != nil || state.CheckEligible() != nil {
		t.Fatalf("ready caller: %+v %v", state, err)
	}
	if _, err := f.profiles.SetSleep(ctx, caller.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.feed.Feed(ctx, caller.ID, domain.PageQuery{Limit: 20}); !errors.Is(err, domain.ErrSleeping) {
		t.Fatalf("repository must recheck sleeping caller: %v", err)
	}
	if _, err := f.profiles.SetSleep(ctx, caller.ID, false); err != nil {
		t.Fatal(err)
	}
	photos, err := f.photos.ByUserID(ctx, caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.photos.Delete(ctx, photos[0].ID, caller.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.feed.Feed(ctx, caller.ID, domain.PageQuery{Limit: 20}); !errors.Is(err, domain.ErrNoPhotos) {
		t.Fatalf("repository must recheck last photo removal: %v", err)
	}
	if _, err := f.states.StateByUserID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown caller: %v", err)
	}
}
