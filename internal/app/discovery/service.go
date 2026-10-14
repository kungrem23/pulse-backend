// Package discovery describes recommendation-feed use cases.
package discovery

import (
	"context"
	"fmt"

	"github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

type Candidate struct {
	Profile domain.Profile
	Photos  []media.Photo
}

type Service interface {
	Feed(context.Context, string, domain.PageRequest) (domain.Page[Candidate], error)
}

type service struct {
	candidates domain.DiscoveryRepository
	states     domain.SearchStateRepository
	storage    domain.PhotoStorage
}

func NewService(candidates domain.DiscoveryRepository, states domain.SearchStateRepository, storage domain.PhotoStorage) Service {
	return &service{candidates: candidates, states: states, storage: storage}
}

func (s *service) Feed(ctx context.Context, userID string, request domain.PageRequest) (domain.Page[Candidate], error) {
	query, err := pagination.Parse(request, pagination.Feed, userID)
	if err != nil {
		return domain.Page[Candidate]{}, err
	}
	state, err := s.states.StateByUserID(ctx, userID)
	if err != nil {
		return domain.Page[Candidate]{}, fmt.Errorf("get feed caller state: %w", err)
	}
	if err := state.CheckEligible(); err != nil {
		return domain.Page[Candidate]{}, err
	}
	result, err := s.candidates.Feed(ctx, userID, query)
	if err != nil {
		return domain.Page[Candidate]{}, fmt.Errorf("get feed candidates: %w", err)
	}
	if len(result.Items) > query.Limit || (result.HasMore && len(result.Items) != query.Limit) {
		return domain.Page[Candidate]{}, fmt.Errorf("feed repository returned an invalid page")
	}
	page := domain.Page[Candidate]{Items: make([]Candidate, 0, len(result.Items))}
	if result.HasMore {
		last := result.Items[len(result.Items)-1]
		page.NextCursor, err = pagination.Encode(domain.PagePosition{CreatedAt: last.CreatedAt, ID: last.Profile.UserID}, pagination.Feed, userID)
		if err != nil {
			return domain.Page[Candidate]{}, fmt.Errorf("encode repository feed position: %v", err)
		}
	}
	for _, item := range result.Items {
		candidate := Candidate{Profile: item.Profile, Photos: make([]media.Photo, 0, len(item.Photos))}
		for _, photo := range item.Photos {
			url, err := s.storage.URL(ctx, photo.ObjectKey)
			if err != nil {
				return domain.Page[Candidate]{}, fmt.Errorf("sign feed photo URL: %w", err)
			}
			candidate.Photos = append(candidate.Photos, media.Photo{ID: photo.ID, URL: url, Position: photo.Position})
		}
		page.Items = append(page.Items, candidate)
	}
	return page, nil
}
