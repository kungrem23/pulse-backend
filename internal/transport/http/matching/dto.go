package matching

import (
	"time"

	appmatching "github.com/kungrem23/pulse-backend/internal/app/matching"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// IsLike is a pointer so that a missing value is rejected instead of being
// saved as a dislike.
type swipeRequest struct {
	ToUserID string `json:"to_user_id"`
	IsLike   *bool  `json:"is_like"`
}

type swipeResponse struct {
	Matched        bool   `json:"matched"`
	MatchID        string `json:"match_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

func newSwipeResponse(match *domain.Match) swipeResponse {
	if match == nil {
		return swipeResponse{}
	}
	response := swipeResponse{Matched: true, MatchID: match.ID}
	if match.ConversationID != nil {
		response.ConversationID = *match.ConversationID
	}
	return response
}

type matchPageResponse struct {
	Items      []matchSummaryResponse `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

type matchSummaryResponse struct {
	ID             string                    `json:"id"`
	Status         string                    `json:"status"`
	CreatedAt      time.Time                 `json:"created_at"`
	ConversationID *string                   `json:"conversation_id,omitempty"`
	User           publicUserSummaryResponse `json:"user"`
}

type publicUserSummaryResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Age      int    `json:"age"`
	PhotoURL string `json:"photo_url,omitempty"`
}

// newMatchPageResponse always returns an items array, [] for an empty page.
func newMatchPageResponse(page domain.Page[appmatching.MatchSummary]) matchPageResponse {
	response := matchPageResponse{
		Items:      make([]matchSummaryResponse, 0, len(page.Items)),
		NextCursor: page.NextCursor,
	}
	for _, match := range page.Items {
		response.Items = append(response.Items, matchSummaryResponse{
			ID:             match.ID,
			Status:         match.Status,
			CreatedAt:      match.CreatedAt,
			ConversationID: match.ConversationID,
			User: publicUserSummaryResponse{
				ID:       match.User.ID,
				Name:     match.User.Name,
				Age:      match.User.Age,
				PhotoURL: match.User.PhotoURL,
			},
		})
	}
	return response
}
