// Package discovery describes recommendation-feed use cases.
package discovery

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type Candidate struct {
	Profile domain.Profile
	Photos  []domain.Photo
}

type Service interface {
	Feed(context.Context, string, int) ([]Candidate, error)
}
