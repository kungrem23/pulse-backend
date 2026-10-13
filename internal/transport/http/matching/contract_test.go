package matching

import (
	"errors"
	"net/http"
	"testing"
	"time"

	appmatching "github.com/kungrem23/pulse-backend/internal/app/matching"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

// Every response of the matching handlers must match api/openapi.yaml.
func TestResponsesMatchOpenAPI(t *testing.T) {
	spec := contracttest.Load(t)
	body := `{"to_user_id":"` + targetID + `","is_like":true}`

	swipeCases := map[string]struct {
		service *serviceStub
		body    string
	}{
		"no match":        {&serviceStub{}, body},
		"match":           {&serviceStub{match: &domain.Match{ID: matchID}}, body},
		"bad body":        {&serviceStub{}, `{"is_like":1}`},
		"self":            {&serviceStub{err: domain.ErrSelfSwipe}, body},
		"invalid target":  {&serviceStub{err: domain.ErrInvalidUserID}, body},
		"sleeping caller": {&serviceStub{err: domain.ErrSleeping}, body},
		"unavailable":     {&serviceStub{err: domain.ErrCandidateUnavailable}, body},
		"conflict":        {&serviceStub{err: domain.ErrConflict}, body},
		"caller gone":     {&serviceStub{err: domain.ErrNotFound}, body},
		"internal":        {&serviceStub{err: errors.New("boom")}, body},
	}
	for name, test := range swipeCases {
		t.Run("swipe "+name, func(t *testing.T) {
			spec.CheckResponse(t, http.MethodPost, "/api/v1/swipes", swipe(test.service, test.body))
		})
	}
	t.Run("swipe not wired", func(t *testing.T) {
		spec.CheckResponse(t, http.MethodPost, "/api/v1/swipes", swipe(nil, body))
	})

	createdAt := time.Date(2026, 10, 13, 12, 0, 0, 123456000, time.UTC)
	page := domain.Page[appmatching.MatchSummary]{
		Items: []appmatching.MatchSummary{
			{ID: matchID, Status: "active", CreatedAt: createdAt, User: appmatching.PublicUserSummary{
				ID: targetID, Name: "Анна", Age: 24, PhotoURL: "https://photos.example/1?sig=x",
			}},
			{ID: targetID, Status: "closed", CreatedAt: createdAt, User: appmatching.PublicUserSummary{ID: callerID, Name: "Борис", Age: 30}},
		},
		NextCursor: "next",
	}
	listCases := map[string]struct {
		service *serviceStub
		query   string
	}{
		"page":           {&serviceStub{page: page}, "?limit=2"},
		"empty":          {&serviceStub{}, ""},
		"bad limit":      {&serviceStub{}, "?limit=0"},
		"invalid cursor": {&serviceStub{err: domain.ErrInvalidCursor}, "?cursor=x"},
		"caller gone":    {&serviceStub{err: domain.ErrNotFound}, ""},
	}
	for name, test := range listCases {
		t.Run("list "+name, func(t *testing.T) {
			spec.CheckResponse(t, http.MethodGet, "/api/v1/matches", serve(test.service, http.MethodGet, "/api/v1/matches"+test.query, ""))
		})
	}
	t.Run("list not wired", func(t *testing.T) {
		spec.CheckResponse(t, http.MethodGet, "/api/v1/matches", serve(nil, http.MethodGet, "/api/v1/matches", ""))
	})
	t.Run("close stays 501", func(t *testing.T) {
		spec.CheckResponse(t, http.MethodDelete, "/api/v1/matches/{match_id}", serve(&serviceStub{}, http.MethodDelete, "/api/v1/matches/"+matchID, ""))
	})
}
