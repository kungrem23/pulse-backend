package discovery

import (
	appdiscovery "github.com/kungrem23/pulse-backend/internal/app/discovery"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

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

func newFeedResponse(page domain.Page[appdiscovery.Candidate]) feedResponse {
	response := feedResponse{Items: make([]candidateResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, candidate := range page.Items {
		profile := candidate.Profile
		item := candidateResponse{UserID: profile.UserID, Name: profile.Name, Bio: profile.Bio,
			Age: profile.Age, Sex: string(profile.Sex), City: profile.City,
			Photos: make([]photoResponse, 0, len(candidate.Photos))}
		for _, photo := range candidate.Photos {
			item.Photos = append(item.Photos, photoResponse{ID: photo.ID, URL: photo.URL, Position: photo.Position})
		}
		response.Items = append(response.Items, item)
	}
	return response
}
