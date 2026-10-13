package discovery

import (
	"errors"
	"net/http"
	"testing"

	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	"github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

func TestFeedResponsesMatchOpenAPI(t *testing.T) {
	spec := contracttest.Load(t)
	page := domain.Page[appdiscovery.Candidate]{Items: []appdiscovery.Candidate{{Profile: domain.Profile{UserID: "22222222-2222-4222-8222-222222222222", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}, Photos: []media.Photo{{ID: "33333333-3333-4333-8333-333333333333", URL: "https://photos.example.test/photo", Position: 1}}}}, NextCursor: "next"}
	spec.CheckResponse(t, http.MethodGet, "/api/v1/feed", serve(&serviceStub{result: page}, "limit=1", "valid"))
	spec.CheckResponse(t, http.MethodGet, "/api/v1/feed", serve(&serviceStub{}, "", "valid"))
	for _, err := range []error{domain.ErrInvalidCursor, domain.ErrInvalidLimit, domain.ErrNotFound, domain.ErrSleeping, domain.ErrProfileIncomplete, domain.ErrNoPhotos, errors.New("internal")} {
		spec.CheckResponse(t, http.MethodGet, "/api/v1/feed", serve(&serviceStub{err: err}, "", "valid"))
	}
	for _, token := range []string{"", "blocked"} {
		spec.CheckResponse(t, http.MethodGet, "/api/v1/feed", serve(&serviceStub{}, "", token))
	}
}
