// Package discovery describes recommendation-feed use cases.
package discovery

import (
	"context"

	"github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

type Candidate struct {
	Profile domain.Profile
	Photos  []media.Photo
}

type Service interface {
	// Feed uses the authenticated user ID and shared pagination validation.
	// Check eligibility through SearchStateRepository; DiscoveryRepository owns
	// mutual filters and exclusions. Resolve photo keys to fresh URLs.
	Feed(context.Context, string, domain.PageRequest) (domain.Page[Candidate], error)
}
