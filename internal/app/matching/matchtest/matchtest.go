// Package matchtest is the behavioural contract of domain.MatchRepository.
// A storage adapter passes it by calling Run from its own test with a Harness
// that seeds users and matches in that storage.
package matchtest

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type UserSpec struct {
	Name           string
	Age            int
	Sex            domain.Sex
	City           string
	Bio            string
	PhotoPositions []int16

	NoPreferences bool
	Sleeping      bool
	Blocked       bool
}

type Harness struct {
	Repository  domain.MatchRepository
	CreateUser  func(t *testing.T, spec UserSpec) string
	CreateMatch func(t *testing.T, a, b string, createdAt time.Time, status string) string
}

var base = time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC)

func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	h := newHarness(t)
	ctx := context.Background()
	person := func(t *testing.T, name string) (string, UserSpec) {
		spec := UserSpec{Name: name, Age: 25, Sex: domain.SexFemale, City: "Москва", Bio: "bio", PhotoPositions: []int16{1}}
		return h.CreateUser(t, spec), spec
	}
	list := func(t *testing.T, userID string, query domain.PageQuery) domain.PageResult[domain.MatchWithPartner] {
		t.Helper()
		result, err := h.Repository.ListByUserID(ctx, userID, query)
		if err != nil {
			t.Fatalf("list matches of %s: %v", userID, err)
		}
		if len(result.Items) > query.Limit {
			t.Fatalf("limit %d exceeded: %d items", query.Limit, len(result.Items))
		}
		return result
	}
	firstPage := domain.PageQuery{Limit: 100}

	t.Run("no matches", func(t *testing.T) {
		a, _ := person(t, "Одиночка")
		result := list(t, a, firstPage)
		if len(result.Items) != 0 || result.HasMore {
			t.Fatalf("expected an empty last page, got %+v", result)
		}
	})

	t.Run("both participants see the pair and the other user", func(t *testing.T) {
		a, _ := person(t, "Анна")
		b, _ := person(t, "Борис")
		stranger, _ := person(t, "Чужой")
		id := h.CreateMatch(t, a, b, base, "active")

		for _, side := range []struct{ owner, partner string }{{a, b}, {b, a}} {
			result := list(t, side.owner, firstPage)
			if len(result.Items) != 1 || result.HasMore {
				t.Fatalf("expected one match, got %+v", result)
			}
			got := result.Items[0]
			checkMatch(t, got.Match, id, a, b, base, "active")
			checkPartner(t, got.Partner, side.partner)
		}
		if result := list(t, stranger, firstPage); len(result.Items) != 0 {
			t.Fatalf("a third user must not see the pair, got %+v", result.Items)
		}
	})

	t.Run("partner data", func(t *testing.T) {
		a, _ := person(t, "Анна")
		spec := UserSpec{Name: "Борис", Age: 31, Sex: domain.SexMale, City: "Казань", Bio: "о себе", PhotoPositions: []int16{3, 1, 5}}
		b := h.CreateUser(t, spec)
		h.CreateMatch(t, a, b, base, "active")

		got := list(t, a, firstPage).Items[0]
		p := got.Partner
		if p.UserID != b || p.Name != spec.Name || p.Age != spec.Age || p.Sex != spec.Sex || p.City != spec.City {
			t.Fatalf("unexpected partner %+v, want %+v", p, spec)
		}
		if got.Photo == nil || got.Photo.UserID != b || got.Photo.Position != 1 || got.Photo.ObjectKey == "" {
			t.Fatalf("expected the partner's photo at the lowest position, got %+v", got.Photo)
		}
	})

	t.Run("existing matches stay visible", func(t *testing.T) {
		owner, _ := person(t, "Владелец")
		partners := map[string]UserSpec{
			"closed":         {},
			"no photos":      {},
			"sleeping":       {Sleeping: true},
			"blocked":        {Blocked: true},
			"no preferences": {NoPreferences: true},
		}
		want := map[string]string{}
		for name, spec := range partners {
			spec.Name, spec.Age, spec.Sex, spec.City = name, 25, domain.SexMale, "Москва"
			if name != "no photos" {
				spec.PhotoPositions = []int16{2}
			}
			partner := h.CreateUser(t, spec)
			status := "active"
			if name == "closed" {
				status = "closed"
			}
			want[h.CreateMatch(t, owner, partner, base, status)] = name
		}

		result := list(t, owner, firstPage)
		if len(result.Items) != len(want) {
			t.Fatalf("expected %d matches, got %d", len(want), len(result.Items))
		}
		for _, item := range result.Items {
			name := want[item.Match.ID]
			if name == "" || item.Partner.Name != name {
				t.Fatalf("unexpected match %+v", item)
			}
			if (item.Match.Status == "closed") != (name == "closed") {
				t.Fatalf("%s: wrong status %q", name, item.Match.Status)
			}
			if (item.Photo == nil) != (name == "no photos") {
				t.Fatalf("%s: unexpected photo %+v", name, item.Photo)
			}
		}
	})

	t.Run("order and pagination", func(t *testing.T) {
		owner, _ := person(t, "Владелец")
		other1, _ := person(t, "Другой 1")
		other2, _ := person(t, "Другой 2")
		type expected struct {
			id        string
			createdAt time.Time
		}
		var all []expected
		for i, offset := range []int{0, 1, 2, 2, 2, 3, 4} {
			partner, _ := person(t, "Партнёр")
			createdAt := base.Add(time.Duration(offset) * time.Second)
			all = append(all, expected{h.CreateMatch(t, owner, partner, createdAt, "active"), createdAt})
			if i%2 == 0 {
				h.CreateMatch(t, other1, other2, createdAt.Add(time.Duration(i)*time.Microsecond), "active")
				other2, _ = person(t, "Другой")
			}
		}
		sort.Slice(all, func(i, j int) bool {
			if !all[i].createdAt.Equal(all[j].createdAt) {
				return all[i].createdAt.After(all[j].createdAt)
			}
			return all[i].id > all[j].id
		})

		for _, limit := range []int{1, 2, 3, 7, 100} {
			var seen []string
			query := domain.PageQuery{Limit: limit}
			for page := 0; ; page++ {
				if page > len(all) {
					t.Fatalf("limit %d: pagination does not end", limit)
				}
				result := list(t, owner, query)
				for _, item := range result.Items {
					seen = append(seen, item.Match.ID)
				}
				if !result.HasMore {
					break
				}
				if len(result.Items) != limit {
					t.Fatalf("limit %d: a page before the last one has %d items", limit, len(result.Items))
				}
				last := result.Items[len(result.Items)-1].Match
				query = domain.PageQuery{Limit: limit, After: &domain.PagePosition{CreatedAt: last.CreatedAt, ID: last.ID}}
			}
			if len(seen) != len(all) {
				t.Fatalf("limit %d: expected %d matches, got %d: %v", limit, len(all), len(seen), seen)
			}
			for i := range all {
				if seen[i] != all[i].id {
					t.Fatalf("limit %d: position %d: expected %s, got %s (order: created_at DESC, id DESC)", limit, i, all[i].id, seen[i])
				}
			}
		}

		oldest := all[len(all)-1]
		result := list(t, owner, domain.PageQuery{Limit: 10, After: &domain.PagePosition{CreatedAt: oldest.createdAt, ID: oldest.id}})
		if len(result.Items) != 0 || result.HasMore {
			t.Fatalf("expected an empty page after the oldest match, got %+v", result)
		}
	})
}

func checkMatch(t *testing.T, match domain.Match, id, a, b string, createdAt time.Time, status string) {
	t.Helper()
	first, second := a, b
	if second < first {
		first, second = second, first
	}
	if match.ID != id || match.Person1ID != first || match.Person2ID != second ||
		!match.CreatedAt.Equal(createdAt) || match.Status != status || match.ConversationID != nil {
		t.Fatalf("unexpected match %+v", match)
	}
}

func checkPartner(t *testing.T, partner domain.Profile, id string) {
	t.Helper()
	if partner.UserID != id || partner.Name == "" || partner.Age == 0 {
		t.Fatalf("expected partner %s, got %+v", id, partner)
	}
}
