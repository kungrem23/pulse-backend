package matching

type swipeRequest struct {
	ToUserID string `json:"to_user_id"`
	IsLike   bool   `json:"is_like"`
}

type swipeResponse struct {
	Matched        bool   `json:"matched"`
	MatchID        string `json:"match_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}
