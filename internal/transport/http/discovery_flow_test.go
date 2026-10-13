package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// Script repository pages; filtering correctness is tested separately on real
// PostgreSQL once SQL is filled, not by duplicating its logic in a test adapter.
type feedFlowRepository struct {
	first, second domain.DiscoveryCandidate
	calls         int
}

func (r *feedFlowRepository) Feed(_ context.Context, _ string, page domain.PageQuery) (domain.PageResult[domain.DiscoveryCandidate], error) {
	r.calls++
	if page.After == nil {
		return domain.PageResult[domain.DiscoveryCandidate]{Items: []domain.DiscoveryCandidate{r.first}, HasMore: true}, nil
	}
	if page.After.ID == r.first.Profile.UserID {
		return domain.PageResult[domain.DiscoveryCandidate]{Items: []domain.DiscoveryCandidate{r.second}}, nil
	}
	return domain.PageResult[domain.DiscoveryCandidate]{Items: []domain.DiscoveryCandidate{}}, nil
}

type feedFlowStates struct {
	photos   domain.PhotoRepository
	sleeping bool
}

func (s *feedFlowStates) StateByUserID(ctx context.Context, owner string) (domain.SearchState, error) {
	photos, err := s.photos.ByUserID(ctx, owner)
	return domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: len(photos), IsSleeping: s.sleeping}, err
}

func TestDiscoveryHTTPFlowAndLastPhotoGate(t *testing.T) {
	photos := &photoFlowRepository{photos: make(map[string]domain.Photo)}
	storage := &photoFlowStorage{objects: make(map[string][]byte)}
	makeCandidate := func(id string) domain.DiscoveryCandidate {
		return domain.DiscoveryCandidate{
			Profile:   domain.Profile{UserID: id, Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Казань", Bio: "Привет"},
			CreatedAt: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
			Photos:    []domain.Photo{{ID: "33333333-3333-4333-8333-333333333333", UserID: id, ObjectKey: "private/photo", Position: 1}},
		}
	}
	repository := &feedFlowRepository{first: makeCandidate("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"), second: makeCandidate("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")}
	states := &feedFlowStates{photos: photos}
	router := NewRouter(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: photoFlowAuth{},
		Media: appmedia.NewService(photos, storage), Discovery: appdiscovery.NewService(repository, states, storage),
	})
	getFeed := func(query string, want int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/feed"+query, nil)
		request.Header.Set("Authorization", "Bearer valid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("expected %d, got %d %s", want, response.Code, response.Body)
		}
		return response
	}
	getFeed("?limit=1", 403)
	if repository.calls != 0 {
		t.Fatal("no-photo caller reached repository")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, photoRequest(t, photoPNG(t), 1, "valid"))
	if response.Code != 201 {
		t.Fatalf("setup upload: %d %s", response.Code, response.Body)
	}
	states.sleeping = true
	getFeed("?limit=1", 403)
	if repository.calls != 0 {
		t.Fatal("sleeping caller reached repository")
	}
	states.sleeping = false
	first := getFeed("?limit=1", 200)
	var page struct {
		Items []struct {
			UserID               string `json:"user_id"`
			Name, Bio, Sex, City string
			Age                  int
			Photos               []struct {
				ID, URL  string
				Position int16
			}
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].UserID != repository.first.Profile.UserID || page.NextCursor == "" || len(page.Items[0].Photos) != 1 {
		t.Fatalf("first public page: %s", first.Body)
	}
	if strings.Contains(first.Body.String(), "private/") || strings.Contains(first.Body.String(), "object_key") {
		t.Fatal("private photo fields leaked")
	}
	second := getFeed("?limit=1&cursor="+page.NextCursor, 200)
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].UserID != repository.second.Profile.UserID || strings.Contains(second.Body.String(), "next_cursor") {
		t.Fatalf("second page must have a different card and no cursor: %s", second.Body)
	}
	getFeed("?cursor=invalid", 400)
	if repository.calls != 2 {
		t.Fatal("invalid cursor reached repository")
	}
	owned, _ := photos.ByUserID(context.Background(), sampleID)
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/photos/"+owned[0].ID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer valid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, deleteRequest)
	if response.Code != 204 {
		t.Fatalf("delete last photo: %d %s", response.Code, response.Body)
	}
	getFeed("?limit=1", 403)
	if repository.calls != 2 {
		t.Fatal("last-photo deletion did not gate discovery")
	}
}
