package media

type uploadResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Position int16  `json:"position"`
}
