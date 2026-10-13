package matching

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	partnerID = "44444444-4444-4444-8444-444444444444"
	strangerA = "55555555-5555-4555-8555-555555555555"
	strangerB = "66666666-6666-4666-8666-666666666666"
	firstID   = "77777777-7777-4777-8777-777777777777"
	secondID  = "88888888-8888-4888-8888-888888888888"
)

type matchRepositoryStub struct {
	result domain.PageResult[domain.MatchWithPartner]
	err    error
	calls  int
	userID string
	query  domain.PageQuery
}

func (s *matchRepositoryStub) ListByUserID(_ context.Context, userID string, query domain.PageQuery) (domain.PageResult[domain.MatchWithPartner], error) {
	s.calls++
	s.userID, s.query = userID, query
	return s.result, s.err
}

type photoStorageStub struct {
	keys []string
	err  error
}

func (s *photoStorageStub) Put(context.Context, string, []byte, string) (string, error) {
	return "", errors.New("unexpected call")
}

func (s *photoStorageStub) Delete(context.Context, string) error {
	return errors.New("unexpected call")
}

func (s *photoStorageStub) URL(_ context.Context, key string) (string, error) {
	s.keys = append(s.keys, key)
	return "https://photos.example/" + key + "?signature=x", s.err
}

var matchTime = time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC)

// entry builds a match of callerID with partner in canonical person order.
func entry(id, partner string, photo *domain.Photo) domain.MatchWithPartner {
	first, second := callerID, partner
	if second < first {
		first, second = second, first
	}
	return domain.MatchWithPartner{
		Match:   domain.Match{ID: id, Person1ID: first, Person2ID: second, Status: "active", CreatedAt: matchTime},
		Partner: domain.Profile{UserID: partner, Name: "Анна", Age: 24, Sex: domain.SexFemale, City: "Москва", Bio: "private bio"},
		Photo:   photo,
	}
}

func listService(matches *matchRepositoryStub, photos *photoStorageStub) Service {
	return NewService(nil, matches, nil, photos)
}

func TestListBuildsPublicSummaries(t *testing.T) {
	photo := &domain.Photo{ID: firstID, UserID: partnerID, ObjectKey: "users/partner/1", Position: 1}
	withoutPhoto := entry(secondID, targetID, nil)
	withoutPhoto.Match.Status = "closed"
	matches := &matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{
		Items: []domain.MatchWithPartner{entry(firstID, partnerID, photo), withoutPhoto},
	}}
	photos := &photoStorageStub{}

	page, err := listService(matches, photos).List(context.Background(), callerID, domain.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if matches.userID != callerID || matches.query.Limit != pagination.DefaultLimit || matches.query.After != nil {
		t.Fatalf("unexpected repository query: %s %+v", matches.userID, matches.query)
	}
	want := []MatchSummary{
		{ID: firstID, Status: "active", CreatedAt: matchTime, User: PublicUserSummary{
			ID: partnerID, Name: "Анна", Age: 24, PhotoURL: "https://photos.example/users/partner/1?signature=x",
		}},
		{ID: secondID, Status: "closed", CreatedAt: matchTime, User: PublicUserSummary{ID: targetID, Name: "Анна", Age: 24}},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("expected %d items, got %+v", len(want), page.Items)
	}
	for i := range want {
		if page.Items[i] != want[i] {
			t.Fatalf("item %d: expected %+v, got %+v", i, want[i], page.Items[i])
		}
	}
	if page.NextCursor != "" {
		t.Fatalf("last page must not have a cursor, got %q", page.NextCursor)
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	page, err := listService(&matchRepositoryStub{}, &photoStorageStub{}).List(context.Background(), callerID, domain.PageRequest{})
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("expected an empty non-nil page, got %#v, %v", page, err)
	}
}

func TestListRejectsInvalidRepositoryPage(t *testing.T) {
	for _, result := range []domain.PageResult[domain.MatchWithPartner]{
		{HasMore: true},
		{Items: []domain.MatchWithPartner{entry(firstID, partnerID, nil)}, HasMore: true},
		{Items: []domain.MatchWithPartner{entry(firstID, partnerID, nil), entry(secondID, targetID, nil), entry(firstID, partnerID, nil)}},
	} {
		page, err := listService(&matchRepositoryStub{result: result}, &photoStorageStub{}).List(context.Background(), callerID, domain.PageRequest{Limit: 2})
		if err == nil || errors.Is(err, domain.ErrInvalidCursor) || len(page.Items) != 0 {
			t.Fatalf("repository corruption must fail as internal error: %+v, %v", page, err)
		}
	}
	invalid := entry("invalid", partnerID, nil)
	_, err := listService(&matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{Items: []domain.MatchWithPartner{invalid}, HasMore: true}}, &photoStorageStub{}).List(context.Background(), callerID, domain.PageRequest{Limit: 1})
	if err == nil || errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("server-created invalid cursor must not become client 400: %v", err)
	}
}

func TestListCursorRoundTrip(t *testing.T) {
	matches := &matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{
		Items:   []domain.MatchWithPartner{entry(firstID, partnerID, nil), entry(secondID, targetID, nil)},
		HasMore: true,
	}}
	service := listService(matches, &photoStorageStub{})

	page, err := service.List(context.Background(), callerID, domain.PageRequest{Limit: 2})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("expected a next cursor, got %+v, %v", page, err)
	}
	matches.result = domain.PageResult[domain.MatchWithPartner]{}
	if _, err := service.List(context.Background(), callerID, domain.PageRequest{Cursor: page.NextCursor, Limit: 2}); err != nil {
		t.Fatal(err)
	}
	after := matches.query.After
	if after == nil || after.ID != secondID || !after.CreatedAt.Equal(matchTime) || matches.query.Limit != 2 {
		t.Fatalf("cursor must point at the last returned match, got %+v", matches.query)
	}
}

func TestListRejectsInvalidPageBeforeRepository(t *testing.T) {
	foreign, err := pagination.Encode(domain.PagePosition{CreatedAt: matchTime, ID: firstID}, pagination.Matches, partnerID)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := pagination.Encode(domain.PagePosition{CreatedAt: matchTime, ID: firstID}, pagination.Feed, callerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		userID  string
		request domain.PageRequest
		want    error
	}{
		{"limit too big", callerID, domain.PageRequest{Limit: 101}, domain.ErrInvalidLimit},
		{"negative limit", callerID, domain.PageRequest{Limit: -1}, domain.ErrInvalidLimit},
		{"garbage cursor", callerID, domain.PageRequest{Cursor: "not a cursor"}, domain.ErrInvalidCursor},
		{"another user's cursor", callerID, domain.PageRequest{Cursor: foreign}, domain.ErrInvalidCursor},
		{"feed cursor", callerID, domain.PageRequest{Cursor: feed}, domain.ErrInvalidCursor},
		{"malformed caller", "user-from-token", domain.PageRequest{}, domain.ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			matches := &matchRepositoryStub{}
			_, err := listService(matches, &photoStorageStub{}).List(context.Background(), test.userID, test.request)
			if !errors.Is(err, test.want) || matches.calls != 0 {
				t.Fatalf("expected %v without a repository call, got %v (calls %d)", test.want, err, matches.calls)
			}
		})
	}
}

func TestListRejectsForeignResults(t *testing.T) {
	foreignPair := entry(firstID, partnerID, nil)
	foreignPair.Match.Person1ID, foreignPair.Match.Person2ID = strangerA, strangerB
	wrongPartner := entry(firstID, partnerID, nil)
	wrongPartner.Partner.UserID = strangerA
	foreignPhoto := entry(firstID, partnerID, &domain.Photo{UserID: strangerA, ObjectKey: "users/stranger/1"})
	selfAsPartner := entry(firstID, partnerID, nil)
	selfAsPartner.Partner.UserID = callerID

	for name, item := range map[string]domain.MatchWithPartner{
		"pair without caller": foreignPair,
		"wrong partner":       wrongPartner,
		"foreign photo":       foreignPhoto,
		"caller as partner":   selfAsPartner,
	} {
		t.Run(name, func(t *testing.T) {
			photos := &photoStorageStub{}
			matches := &matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{Items: []domain.MatchWithPartner{item}}}
			page, err := listService(matches, photos).List(context.Background(), callerID, domain.PageRequest{})
			if err == nil || len(page.Items) != 0 || len(photos.keys) != 0 {
				t.Fatalf("foreign data must fail the request, got %+v, %v", page, err)
			}
		})
	}
}

func TestListFailsOnRepositoryAndStorageErrors(t *testing.T) {
	failure := errors.New("boom")
	if _, err := listService(&matchRepositoryStub{err: failure}, &photoStorageStub{}).List(
		context.Background(), callerID, domain.PageRequest{},
	); !errors.Is(err, failure) {
		t.Fatalf("expected repository error, got %v", err)
	}

	photo := &domain.Photo{UserID: partnerID, ObjectKey: "users/partner/1"}
	matches := &matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{
		Items: []domain.MatchWithPartner{entry(firstID, partnerID, photo)},
	}}
	if _, err := listService(matches, &photoStorageStub{err: failure}).List(
		context.Background(), callerID, domain.PageRequest{},
	); !errors.Is(err, failure) {
		t.Fatalf("expected storage error, got %v", err)
	}

	tooMany := &matchRepositoryStub{result: domain.PageResult[domain.MatchWithPartner]{
		Items: []domain.MatchWithPartner{entry(firstID, partnerID, nil), entry(secondID, targetID, nil)},
	}}
	if _, err := listService(tooMany, &photoStorageStub{}).List(
		context.Background(), callerID, domain.PageRequest{Limit: 1},
	); err == nil {
		t.Fatal("more items than the limit must be an error")
	}
}
