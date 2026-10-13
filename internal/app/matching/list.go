package matching

import (
	"context"
	"errors"
	"fmt"

	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// List returns the caller's matches, newest first. Errors: ErrNotFound for a
// caller ID that cannot identify a user, ErrInvalidCursor and ErrInvalidLimit.
func (s *service) List(ctx context.Context, userID string, request domain.PageRequest) (domain.Page[MatchSummary], error) {
	owner, ok := canonicalUUID(userID)
	if !ok {
		return domain.Page[MatchSummary]{}, fmt.Errorf("list matches: caller: %w", domain.ErrNotFound)
	}
	query, err := pagination.Parse(request, pagination.Matches, owner)
	if err != nil {
		return domain.Page[MatchSummary]{}, fmt.Errorf("list matches: %w", err)
	}
	result, err := s.matches.ListByUserID(ctx, owner, query)
	if err != nil {
		return domain.Page[MatchSummary]{}, fmt.Errorf("list matches: %w", err)
	}
	if len(result.Items) > query.Limit || (result.HasMore && len(result.Items) != query.Limit) {
		return domain.Page[MatchSummary]{}, errors.New("list matches: repository returned an invalid page")
	}

	page := domain.Page[MatchSummary]{Items: make([]MatchSummary, 0, len(result.Items))}
	for _, entry := range result.Items {
		summary, err := s.summary(ctx, owner, entry)
		if err != nil {
			return domain.Page[MatchSummary]{}, fmt.Errorf("list matches: %w", err)
		}
		page.Items = append(page.Items, summary)
	}
	if result.HasMore && len(result.Items) > 0 {
		last := result.Items[len(result.Items)-1].Match
		cursor, err := pagination.Encode(domain.PagePosition{CreatedAt: last.CreatedAt, ID: last.ID}, pagination.Matches, owner)
		if err != nil {
			return domain.Page[MatchSummary]{}, fmt.Errorf("list matches: repository next cursor: %v", err)
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// summary keeps only public partner data. A result that does not belong to the
// owner is an internal error, so a faulty query cannot leak someone else's pair.
func (s *service) summary(ctx context.Context, owner string, entry domain.MatchWithPartner) (MatchSummary, error) {
	match := entry.Match
	var partnerID string
	switch owner {
	case match.Person1ID:
		partnerID = match.Person2ID
	case match.Person2ID:
		partnerID = match.Person1ID
	default:
		return MatchSummary{}, fmt.Errorf("match %s does not belong to the caller", match.ID)
	}
	if entry.Partner.UserID != partnerID {
		return MatchSummary{}, fmt.Errorf("match %s has a wrong partner", match.ID)
	}

	partner := PublicUserSummary{ID: partnerID, Name: entry.Partner.Name, Age: entry.Partner.Age}
	if entry.Photo != nil {
		if entry.Photo.UserID != partnerID {
			return MatchSummary{}, fmt.Errorf("match %s has a photo of another user", match.ID)
		}
		if s.photos == nil {
			return MatchSummary{}, errors.New("photo storage is not configured")
		}
		url, err := s.photos.URL(ctx, entry.Photo.ObjectKey)
		if err != nil {
			return MatchSummary{}, fmt.Errorf("photo URL: %w", err)
		}
		partner.PhotoURL = url
	}
	// ConversationID stays nil in MVP: chat is not created with a match.
	return MatchSummary{ID: match.ID, Status: match.Status, CreatedAt: match.CreatedAt, User: partner}, nil
}
