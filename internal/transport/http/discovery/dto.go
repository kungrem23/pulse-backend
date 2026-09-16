package discovery

type feedResponse struct {
	Items      []candidateResponse `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type candidateResponse struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Bio    string   `json:"bio"`
	Age    int      `json:"age"`
	Photos []string `json:"photos"`
}
