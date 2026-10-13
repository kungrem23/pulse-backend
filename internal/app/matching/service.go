// Package matching describes swipe and match use cases.
package matching

import (
	"context"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type SwipeCommand struct {
	// FromUserID comes from the verified access JWT, never the request JSON.
	FromUserID string
	ToUserID   string
	IsLike     bool
}

// PublicUserSummary contains only partner data allowed by OpenAPI.
// PhotoURL is empty when the partner no longer has a photo.
type PublicUserSummary struct {
	ID       string
	Name     string
	Age      int
	PhotoURL string
}

// MatchSummary is the application result for a participant's match list.
// ConversationID remains nil in MVP; chat fields are not required.
type MatchSummary struct {
	ID             string
	Status         string
	CreatedAt      time.Time
	ConversationID *string
	User           PublicUserSummary
}

type Service interface {
	// Validate UUIDs and self-swipe, then precheck the caller via
	// SearchStateRepository. SwipeRepository.Save repeats state/filter checks
	// atomically; do not depend on the discovery service to validate a target.
	Swipe(context.Context, SwipeCommand) (*domain.Match, error)
	// List uses MatchRepository plus PhotoStorage to assemble public partner
	// summaries; sleeping/incomplete profiles can still read existing matches.
	List(context.Context, string, domain.PageRequest) (domain.Page[MatchSummary], error)
	// Close is post-MVP. The pair remains permanently excluded from discovery.
	Close(context.Context, string, string) error
}
