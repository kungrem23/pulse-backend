package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

// TODO(db): SQL is written by the database developer. Keep the parameter order
// and the returned column order: the Go code scans results positionally.
const (
	// Params: $1 user_id, $2 age.
	// Sets users.age and users.updated_at. Zero affected rows means no such user.
	updateUserAgeQuery = ""

	// Params: $1 user_id, $2 name, $3 bio, $4 sex, $5 city.
	// Creates the profile or replaces these fields of an existing one in a single
	// statement (concurrent first saves must not fail with unique_violation).
	// Must not change is_sleeping of an existing profile; sets updated_at.
	// Returns one row: is_sleeping.
	upsertProfileQuery = ""

	// Params: $1 user_id.
	// Returns zero or one row: user_id (UUID as text), name, bio, age (from users), sex, city, is_sleeping.
	profileByUserIDQuery = ""

	// Params: $1 user_id.
	// Returns zero or one row: user_id (UUID as text), min_age, max_age, sex.
	preferencesByUserIDQuery = ""
)

type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

func (r *ProfileRepository) Save(ctx context.Context, profile domain.Profile) (domain.Profile, error) {
	if updateUserAgeQuery == "" || upsertProfileQuery == "" {
		return domain.Profile{}, fmt.Errorf("save profile: %w", errQueryNotWritten)
	}
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
	if profileByUserIDQuery == "" {
		return domain.Profile{}, fmt.Errorf("find profile: %w", errQueryNotWritten)
	}
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
	if preferencesByUserIDQuery == "" {
		return domain.Preferences{}, fmt.Errorf("find preferences: %w", errQueryNotWritten)
	}
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

// notFoundOr maps a missing row or a malformed UUID to domain.ErrNotFound.
func notFoundOr(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) || pgErrorCode(err) == invalidTextRepresentation {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
