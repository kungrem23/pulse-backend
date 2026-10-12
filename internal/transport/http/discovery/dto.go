package discovery

type feedResponse struct {
	Items      []candidateResponse `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type candidateResponse struct {
	UserID string          `json:"user_id"`
	Name   string          `json:"name"`
	Bio    string          `json:"bio"`
	Age    int             `json:"age"`
	Sex    string          `json:"sex"`
	City   string          `json:"city"`
	Photos []photoResponse `json:"photos"`
}

type photoResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Position int16  `json:"position"`
}
