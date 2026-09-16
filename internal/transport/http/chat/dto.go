package chat

type sendRequest struct {
	Text      string  `json:"text"`
	ReplyToID *string `json:"reply_to_id,omitempty"`
}
