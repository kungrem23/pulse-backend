package discovery

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	callerID = "11111111-1111-4111-8111-111111111111"
	firstID  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	lastID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

type stateStub struct {
	state domain.SearchState
	err   error
	calls int
	owner string
}

func (s *stateStub) StateByUserID(_ context.Context, owner string) (domain.SearchState, error) {
	s.calls++
	s.owner = owner
	return s.state, s.err
}

type candidatesStub struct {
	result domain.PageResult[domain.DiscoveryCandidate]
	err    error
	calls  int
	owner  string
	query  domain.PageQuery
}

func (s *candidatesStub) Feed(_ context.Context, owner string, query domain.PageQuery) (domain.PageResult[domain.DiscoveryCandidate], error) {
	s.calls++
	s.owner, s.query = owner, query
	return s.result, s.err
}

type storageStub struct {
	domain.PhotoStorage
	keys []string
	err  error
}

func (s *storageStub) URL(_ context.Context, key string) (string, error) {
	s.keys = append(s.keys, key)
	return "https://photos.example.test/fresh/" + strconv.Itoa(len(s.keys)), s.err
}

func readyState() domain.SearchState {
	return domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: 1}
}

func candidate(id string) domain.DiscoveryCandidate {
	return domain.DiscoveryCandidate{
		Profile:   domain.Profile{UserID: id, Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Казань", Bio: "Привет"},
		CreatedAt: time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC),
		Photos: []domain.Photo{
			{ID: "22222222-2222-4222-8222-222222222222", UserID: id, ObjectKey: "private/first", Position: 1},
			{ID: "33333333-3333-4333-8333-333333333333", UserID: id, ObjectKey: "private/third", Position: 3},
		},
	}
}

func TestFeedSignsPhotosAndUsesLastReturnedCursor(t *testing.T) {
	repository := &candidatesStub{result: domain.PageResult[domain.DiscoveryCandidate]{
		Items: []domain.DiscoveryCandidate{candidate(firstID), candidate(lastID)}, HasMore: true,
	}}
	states := &stateStub{state: readyState()}
	storage := &storageStub{}
	service := NewService(repository, states, storage)
	page, err := service.Feed(context.Background(), callerID, domain.PageRequest{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" || states.owner != callerID || repository.owner != callerID {
		t.Fatalf("unexpected feed: %+v, %v", page, err)
	}
	if len(storage.keys) != 4 || storage.keys[0] != "private/first" || storage.keys[1] != "private/third" ||
		page.Items[0].Photos[0].URL != "https://photos.example.test/fresh/1" || page.Items[0].Photos[1].Position != 3 {
		t.Fatalf("photo order or signing is wrong: %+v", page.Items)
	}
	decoded, err := pagination.Parse(domain.PageRequest{Cursor: page.NextCursor, Limit: 2}, pagination.Feed, callerID)
	if err != nil || decoded.After == nil || decoded.After.ID != lastID || !decoded.After.CreatedAt.Equal(candidate(lastID).CreatedAt) {
		t.Fatalf("next cursor must refer to the last returned row: %+v, %v", decoded, err)
	}
	repository.result = domain.PageResult[domain.DiscoveryCandidate]{Items: []domain.DiscoveryCandidate{candidate(lastID)}}
	next, err := service.Feed(context.Background(), callerID, domain.PageRequest{Cursor: page.NextCursor, Limit: 2})
	if err != nil || next.NextCursor != "" || repository.query.After == nil || repository.query.After.ID != lastID {
		t.Fatalf("decoded boundary was not passed to repository: %+v, %v", repository.query, err)
	}
	if len(storage.keys) != 6 || next.Items[0].Photos[0].URL == page.Items[1].Photos[0].URL {
		t.Fatal("each request must obtain fresh URLs")
	}
}

func TestFeedEmptyAndDefaultLimit(t *testing.T) {
	repository := &candidatesStub{}
	page, err := NewService(repository, &stateStub{state: readyState()}, &storageStub{}).Feed(context.Background(), callerID, domain.PageRequest{})
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" || repository.query.Limit != 20 {
		t.Fatalf("expected default limit and empty non-nil page: %+v, %v", page, err)
	}
}

func TestFeedRejectsInputBeforeRepositories(t *testing.T) {
	foreignCursor, err := pagination.Encode(domain.PagePosition{CreatedAt: time.Now(), ID: firstID}, pagination.Feed, lastID)
	if err != nil {
		t.Fatal(err)
	}
	matchCursor, err := pagination.Encode(domain.PagePosition{CreatedAt: time.Now(), ID: firstID}, pagination.Matches, callerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		owner string
		page  domain.PageRequest
		want  error
	}{
		{"invalid", domain.PageRequest{}, domain.ErrInvalidUserID},
		{callerID, domain.PageRequest{Limit: -1}, domain.ErrInvalidLimit},
		{callerID, domain.PageRequest{Limit: 101}, domain.ErrInvalidLimit},
		{callerID, domain.PageRequest{Cursor: "invalid"}, domain.ErrInvalidCursor},
		{callerID, domain.PageRequest{Cursor: foreignCursor}, domain.ErrInvalidCursor},
		{callerID, domain.PageRequest{Cursor: matchCursor}, domain.ErrInvalidCursor},
	} {
		repository := &candidatesStub{}
		states := &stateStub{state: readyState()}
		_, err := NewService(repository, states, &storageStub{}).Feed(context.Background(), test.owner, test.page)
		if !errors.Is(err, test.want) || states.calls != 0 || repository.calls != 0 {
			t.Fatalf("expected %v before DB calls, got %v", test.want, err)
		}
	}
}

func TestFeedEligibilityGatesAndPrecheckRace(t *testing.T) {
	for _, test := range []struct {
		state domain.SearchState
		want  error
	}{
		{domain.SearchState{}, domain.ErrProfileIncomplete},
		{domain.SearchState{HasProfile: true, PhotoCount: 1}, domain.ErrProfileIncomplete},
		{domain.SearchState{HasProfile: true, HasPreferences: true}, domain.ErrNoPhotos},
		{domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: 1, IsSleeping: true}, domain.ErrSleeping},
		{domain.SearchState{IsBlocked: true}, domain.ErrBlocked},
	} {
		repository := &candidatesStub{}
		_, err := NewService(repository, &stateStub{state: test.state}, &storageStub{}).Feed(context.Background(), callerID, domain.PageRequest{})
		if !errors.Is(err, test.want) || repository.calls != 0 {
			t.Fatalf("expected %v before feed read, got %v", test.want, err)
		}
	}
	repository := &candidatesStub{err: domain.ErrNoPhotos}
	storage := &storageStub{}
	_, err := NewService(repository, &stateStub{state: readyState()}, storage).Feed(context.Background(), callerID, domain.PageRequest{})
	if !errors.Is(err, domain.ErrNoPhotos) || repository.calls != 1 || len(storage.keys) != 0 {
		t.Fatalf("repository recheck must reject stale eligibility: %v", err)
	}
}

func TestFeedStorageErrorsAndInvalidRepositoryPage(t *testing.T) {
	failure := errors.New("private backend error")
	repository := &candidatesStub{result: domain.PageResult[domain.DiscoveryCandidate]{Items: []domain.DiscoveryCandidate{candidate(firstID)}}}
	states := &stateStub{state: readyState(), err: failure}
	storage := &storageStub{}
	service := NewService(repository, states, storage)
	if _, err := service.Feed(context.Background(), callerID, domain.PageRequest{}); !errors.Is(err, failure) || repository.calls != 0 {
		t.Fatalf("state error: %v", err)
	}
	states.err = domain.ErrNotFound
	if _, err := service.Feed(context.Background(), callerID, domain.PageRequest{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing caller: %v", err)
	}
	states.err = nil
	repository.err = failure
	if _, err := service.Feed(context.Background(), callerID, domain.PageRequest{}); !errors.Is(err, failure) || len(storage.keys) != 0 {
		t.Fatalf("repository error: %v", err)
	}
	repository.err = nil
	storage.err = failure
	if _, err := service.Feed(context.Background(), callerID, domain.PageRequest{}); !errors.Is(err, failure) {
		t.Fatalf("signing error: %v", err)
	}
	storage.err = nil
	for _, result := range []domain.PageResult[domain.DiscoveryCandidate]{
		{HasMore: true},
		{Items: []domain.DiscoveryCandidate{candidate(firstID)}, HasMore: true},
		{Items: []domain.DiscoveryCandidate{candidate(firstID), candidate(lastID), candidate(firstID)}},
		{Items: []domain.DiscoveryCandidate{{}, {}}, HasMore: true},
	} {
		repository.result = result
		if _, err := service.Feed(context.Background(), callerID, domain.PageRequest{Limit: 2}); err == nil || errors.Is(err, domain.ErrInvalidCursor) {
			t.Fatalf("bad repository page must produce an internal error: %v", err)
		}
	}
}
