package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/app/matching/matchtest"
	"github.com/kungrem23/pulse-backend/internal/app/matching/swipetest"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

// Only pgtest's throwaway database may run these fixture queries; never point the harness at the application database.
const (
	// $1 user UUID. Set this user's is_blocked to true.
	matchingTestBlockUserQuery = `
UPDATE users SET is_blocked = true, updated_at = now() WHERE id = $1::uuid
`
	// $1 from UUID, $2 to UUID. Return is_like, or no rows if absent.
	matchingTestSwipeQuery = `
SELECT is_like FROM swipes WHERE from_user_id = $1::uuid AND to_user_id = $2::uuid
`
	// $1 canonical person1 UUID, $2 person2 UUID. All pair matches, any status.
	// Columns: id::text, person1_id::text, person2_id::text, status, created_at.
	matchingTestPairMatchesQuery = `
SELECT id::text, person1_id::text, person2_id::text, status, created_at
FROM matches WHERE person1_id = $1::uuid AND person2_id = $2::uuid
`
	// $1 canonical person1 UUID, $2 person2 UUID, $3 created_at, $4 status.
	// Insert pair, set closed_at coherently (closed vs active), return id::text.
	matchingTestCreateMatchQuery = `
INSERT INTO matches (person1_id, person2_id, created_at, status, closed_at)
VALUES ($1::uuid, $2::uuid, $3::timestamptz, $4,
    CASE WHEN $4 = 'closed' THEN $3::timestamptz ELSE NULL END)
RETURNING id::text
`
)

type matchingFixture struct{ pool *pgxpool.Pool }

func (f matchingFixture) createUser(t *testing.T, spec matchtest.UserSpec, noProfile bool, min, max int16, searchSex domain.SearchSex) string {
	t.Helper()
	ctx := context.Background()
	user, err := postgres.NewUserRepository(f.pool).Create(ctx, domain.User{Email: uuid.NewString() + "@example.test", PasswordHash: "test-hash"})
	if err != nil {
		t.Fatal(err)
	}
	profiles := postgres.NewProfileRepository(f.pool)
	if !noProfile {
		_, err = profiles.Save(ctx, domain.Profile{UserID: user.ID, Name: spec.Name, Age: spec.Age, Sex: spec.Sex, City: spec.City, Bio: spec.Bio})
		if err != nil {
			t.Fatal(err)
		}
		if !spec.NoPreferences {
			if _, err := profiles.SavePreferences(ctx, domain.Preferences{UserID: user.ID, MinAge: min, MaxAge: max, Sex: searchSex}); err != nil {
				t.Fatal(err)
			}
		}
		if spec.Sleeping {
			if _, err := profiles.SetSleep(ctx, user.ID, true); err != nil {
				t.Fatal(err)
			}
		}
		for _, position := range spec.PhotoPositions {
			id := uuid.NewString()
			if err := postgres.NewPhotoRepository(f.pool).Create(ctx, domain.Photo{ID: id, UserID: user.ID, ObjectKey: "test/" + id, Position: position}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if spec.Blocked {
		if _, err := f.pool.Exec(ctx, matchingTestBlockUserQuery, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	return user.ID
}

func TestSwipeRepositoryContractWithPostgres(t *testing.T) {
	swipetest.Run(t, func(t *testing.T) swipetest.Harness {
		f := matchingFixture{pool: pgtest.NewDatabase(t)}
		return swipetest.Harness{
			Repository: postgres.NewSwipeRepository(f.pool),
			CreateUser: func(t *testing.T, spec swipetest.UserSpec) string {
				if spec.Age == 0 {
					spec.Age = 25
				}
				if spec.Sex == "" {
					spec.Sex = domain.SexMale
				}
				if spec.MinAge == 0 {
					spec.MinAge = 18
				}
				if spec.MaxAge == 0 {
					spec.MaxAge = 120
				}
				if spec.SearchSex == "" {
					spec.SearchSex = domain.SearchSexAny
				}
				person := matchtest.UserSpec{Name: "Пользователь", Age: spec.Age, Sex: spec.Sex, City: "Москва",
					NoPreferences: spec.NoPreferences, Sleeping: spec.Sleeping, Blocked: spec.Blocked}
				if !spec.NoPhotos {
					person.PhotoPositions = []int16{1}
				}
				return f.createUser(t, person, spec.NoProfile, int16(spec.MinAge), int16(spec.MaxAge), spec.SearchSex)
			},
			Swipe: func(t *testing.T, from, to string) (bool, bool) {
				t.Helper()
				var like bool
				err := f.pool.QueryRow(context.Background(), matchingTestSwipeQuery, from, to).Scan(&like)
				if errors.Is(err, pgx.ErrNoRows) {
					return false, false
				}
				if err != nil {
					t.Fatal(err)
				}
				return like, true
			},
			Matches: func(t *testing.T, a, b string) []domain.Match {
				t.Helper()
				if a > b {
					a, b = b, a
				}
				rows, err := f.pool.Query(context.Background(), matchingTestPairMatchesQuery, a, b)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				matches := make([]domain.Match, 0)
				for rows.Next() {
					var match domain.Match
					if err := rows.Scan(&match.ID, &match.Person1ID, &match.Person2ID, &match.Status, &match.CreatedAt); err != nil {
						t.Fatal(err)
					}
					matches = append(matches, match)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				return matches
			},
		}
	})
}

func TestMatchRepositoryContractWithPostgres(t *testing.T) {
	matchtest.Run(t, func(t *testing.T) matchtest.Harness {
		f := matchingFixture{pool: pgtest.NewDatabase(t)}
		return matchtest.Harness{
			Repository: postgres.NewMatchRepository(f.pool),
			CreateUser: func(t *testing.T, spec matchtest.UserSpec) string {
				return f.createUser(t, spec, false, 18, 120, domain.SearchSexAny)
			},
			CreateMatch: func(t *testing.T, a, b string, createdAt time.Time, status string) string {
				t.Helper()
				if a > b {
					a, b = b, a
				}
				var id string
				if err := f.pool.QueryRow(context.Background(), matchingTestCreateMatchQuery, a, b, createdAt, status).Scan(&id); err != nil {
					t.Fatal(err)
				}
				return id
			},
		}
	})
}
