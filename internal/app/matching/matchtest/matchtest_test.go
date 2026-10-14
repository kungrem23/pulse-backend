package matchtest

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type memoryRepository struct {
	mu      sync.Mutex
	users   map[string]UserSpec
	matches []domain.Match
}

func (r *memoryRepository) ListByUserID(_ context.Context, userID string, query domain.PageQuery) (domain.PageResult[domain.MatchWithPartner], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var own []domain.Match
	for _, match := range r.matches {
		if match.Person1ID != userID && match.Person2ID != userID {
			continue
		}
		if after := query.After; after != nil {
			if match.CreatedAt.After(after.CreatedAt) ||
				(match.CreatedAt.Equal(after.CreatedAt) && match.ID >= after.ID) {
				continue
			}
		}
		own = append(own, match)
	}
	sort.Slice(own, func(i, j int) bool {
		if !own[i].CreatedAt.Equal(own[j].CreatedAt) {
			return own[i].CreatedAt.After(own[j].CreatedAt)
		}
		return own[i].ID > own[j].ID
	})

	result := domain.PageResult[domain.MatchWithPartner]{}
	if len(own) > query.Limit {
		own, result.HasMore = own[:query.Limit], true
	}
	for _, match := range own {
		partnerID := match.Person1ID
		if partnerID == userID {
			partnerID = match.Person2ID
		}
		spec := r.users[partnerID]
		item := domain.MatchWithPartner{
			Match:   match,
			Partner: domain.Profile{UserID: partnerID, Name: spec.Name, Age: spec.Age, Sex: spec.Sex, City: spec.City, Bio: spec.Bio, IsSleeping: spec.Sleeping},
		}
		if len(spec.PhotoPositions) > 0 {
			lowest := spec.PhotoPositions[0]
			for _, position := range spec.PhotoPositions {
				lowest = min(lowest, position)
			}
			item.Photo = &domain.Photo{ID: uuid.NewString(), UserID: partnerID, ObjectKey: "users/" + partnerID, Position: lowest}
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func TestMemoryRepositorySatisfiesContract(t *testing.T) {
	Run(t, func(t *testing.T) Harness {
		repository := &memoryRepository{users: map[string]UserSpec{}}
		return Harness{
			Repository: repository,
			CreateUser: func(_ *testing.T, spec UserSpec) string {
				id := uuid.NewString()
				repository.mu.Lock()
				defer repository.mu.Unlock()
				repository.users[id] = spec
				return id
			},
			CreateMatch: func(_ *testing.T, a, b string, createdAt time.Time, status string) string {
				if b < a {
					a, b = b, a
				}
				match := domain.Match{ID: uuid.NewString(), Person1ID: a, Person2ID: b, Status: status, CreatedAt: createdAt}
				repository.mu.Lock()
				defer repository.mu.Unlock()
				repository.matches = append(repository.matches, match)
				return match.ID
			},
		}
	})
}
