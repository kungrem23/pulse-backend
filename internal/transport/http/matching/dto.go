package matching

import "time"

type swipeRequest struct {
	ToUserID string `json:"to_user_id"`
	IsLike   bool   `json:"is_like"`
}

type swipeResponse struct {
	Matched        bool   `json:"matched"`
	MatchID        string `json:"match_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
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
