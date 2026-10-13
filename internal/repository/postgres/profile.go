package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

// Parameter and result column order follow the repository contract.
// The Go code scans results positionally.
const (
	// Params: $1 user_id, $2 age.
	// Sets users.age and users.updated_at. Zero affected rows means no such user.
	updateUserAgeQuery = `
UPDATE users SET age = $2, updated_at = now()
WHERE id = $1::uuid
`

	// Params: $1 user_id, $2 name, $3 bio, $4 sex, $5 city.
	// Creates the profile or replaces these fields of an existing one in a single
	// statement (concurrent first saves must not fail with unique_violation).
	// Must not change is_sleeping of an existing profile; sets updated_at.
	// Returns one row: is_sleeping.
	upsertProfileQuery = `
INSERT INTO profiles (user_id, name, bio, sex, city)
VALUES ($1::uuid, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE
SET name = EXCLUDED.name, bio = EXCLUDED.bio, sex = EXCLUDED.sex,
    city = EXCLUDED.city, updated_at = now()
RETURNING is_sleeping
`

	// Params: $1 user_id.
	// Returns zero or one row: user_id (UUID as text), name, bio, age (from users), sex, city, is_sleeping.
	profileByUserIDQuery = `
SELECT p.user_id::text, p.name, p.bio, u.age, p.sex, p.city, p.is_sleeping
FROM profiles p JOIN users u ON u.id = p.user_id
WHERE p.user_id = $1::uuid
`

	// Params: $1 user_id.
	// Returns zero or one row: user_id (UUID as text), min_age, max_age, sex.
	preferencesByUserIDQuery = `
SELECT user_id::text, min_age, max_age, sex
FROM preferences WHERE user_id = $1::uuid
`

	// Params: $1 user_id, $2 min_age, $3 max_age, $4 sex.
	// Creates or replaces the user's preferences in a single statement.
	// Without a profile it must fail with foreign_key_violation (23503).
	// Returns one row: user_id (UUID as text), min_age, max_age, sex.
	upsertPreferencesQuery = `
INSERT INTO preferences (user_id, min_age, max_age, sex)
VALUES ($1::uuid, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET min_age = EXCLUDED.min_age, max_age = EXCLUDED.max_age, sex = EXCLUDED.sex
RETURNING user_id::text, min_age, max_age, sex
`

	// Params: $1 user_id, $2 is_sleeping.
	// Sets profiles.is_sleeping and updated_at. Returns zero rows if there is
	// no profile, otherwise one row: is_sleeping.
	setSleepQuery = `
UPDATE profiles SET is_sleeping = $2, updated_at = now()
WHERE user_id = $1::uuid
RETURNING is_sleeping
`
)

type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

func (r *ProfileRepository) Save(ctx context.Context, profile domain.Profile) (domain.Profile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("save profile: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, updateUserAgeQuery, profile.UserID, profile.Age)
	if err != nil {
		if pgErrorCode(err) == invalidTextRepresentation {
			return domain.Profile{}, fmt.Errorf("save profile: %w", domain.ErrNotFound)
		}
		return domain.Profile{}, fmt.Errorf("save profile: update age: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.Profile{}, fmt.Errorf("save profile: %w", domain.ErrNotFound)
	}
	err = tx.QueryRow(ctx, upsertProfileQuery,
		profile.UserID, profile.Name, profile.Bio, string(profile.Sex), profile.City,
	).Scan(&profile.IsSleeping)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("save profile: upsert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Profile{}, fmt.Errorf("save profile: commit: %w", err)
	}
	return profile, nil
}

func (r *ProfileRepository) ByUserID(ctx context.Context, userID string) (domain.Profile, error) {
	var profile domain.Profile
	var sex string
	err := r.pool.QueryRow(ctx, profileByUserIDQuery, userID).Scan(
		&profile.UserID, &profile.Name, &profile.Bio, &profile.Age, &sex, &profile.City, &profile.IsSleeping,
	)
	if err != nil {
		return domain.Profile{}, notFoundOr("find profile", err)
	}
	profile.Sex = domain.Sex(sex)
	return profile, nil
}

func (r *ProfileRepository) Preferences(ctx context.Context, userID string) (domain.Preferences, error) {
	var preferences domain.Preferences
	var sex string
	err := r.pool.QueryRow(ctx, preferencesByUserIDQuery, userID).Scan(
		&preferences.UserID, &preferences.MinAge, &preferences.MaxAge, &sex,
	)
	if err != nil {
		return domain.Preferences{}, notFoundOr("find preferences", err)
	}
	preferences.Sex = domain.SearchSex(sex)
	return preferences, nil
}

func (r *ProfileRepository) SavePreferences(ctx context.Context, preferences domain.Preferences) (domain.Preferences, error) {
	var saved domain.Preferences
	var sex string
	err := r.pool.QueryRow(ctx, upsertPreferencesQuery,
		preferences.UserID, preferences.MinAge, preferences.MaxAge, string(preferences.Sex),
	).Scan(&saved.UserID, &saved.MinAge, &saved.MaxAge, &sex)
	if err != nil {
		if pgErrorCode(err) == foreignKeyViolation {
			return domain.Preferences{}, fmt.Errorf("save preferences: %w", domain.ErrNotFound)
		}
		return domain.Preferences{}, notFoundOr("save preferences", err)
	}
	saved.Sex = domain.SearchSex(sex)
	return saved, nil
}

func (r *ProfileRepository) SetSleep(ctx context.Context, userID string, isSleeping bool) (bool, error) {
	var stored bool
	if err := r.pool.QueryRow(ctx, setSleepQuery, userID, isSleeping).Scan(&stored); err != nil {
		return false, notFoundOr("set sleep mode", err)
	}
	return stored, nil
}

// notFoundOr maps a missing row or a malformed UUID to domain.ErrNotFound.
func notFoundOr(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) || pgErrorCode(err) == invalidTextRepresentation {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
