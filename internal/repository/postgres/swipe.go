package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// TODO(db): only query placeholders are intentionally unfinished. All locks are
// held until commit. Lock users (including absent-profile users), then profiles,
// preferences and existing photos in canonical user order. These resource locks
// serialize overlapping pairs and protect block/age/sleep/preferences/photo state.
// Use row locks that conflict with UPDATE/DELETE; do not use just a match UNIQUE.
const (
	// $1 ordered array of two user UUIDs. Lock users ORDER BY id FOR UPDATE.
	lockSwipeUsersQuery = ""
	// $1 same UUID array. Lock profiles ORDER BY user_id FOR UPDATE.
	lockSwipeProfilesQuery = ""
	// $1 same UUID array. Lock preferences ORDER BY user_id FOR UPDATE.
	lockSwipePreferencesQuery = ""
	// $1 same UUID array. Lock photos ORDER BY user_id, position FOR UPDATE.
	// This protects deletion of the last photo; do not replace with unlocked count.
	lockSwipePhotosQuery = ""
	// $1 from UUID, $2 to UUID. Return one bool: outgoing swipe OR ANY existing
	// match pair in either ordering/status. No resets; a closed pair still conflicts.
	swipeHistoryConflictQuery = ""
	// $1 from UUID, $2 to UUID. Return one bool: all four mutual age/sex checks,
	// including any in either preference. No city filter. Missing data -> false.
	swipeMutualPreferencesQuery = ""
	// $1 from UUID, $2 to UUID, $3 is_like. INSERT exactly once, no upsert.
	insertSwipeQuery = ""
	// $1 from UUID, $2 to UUID. Return one bool: to -> from has is_like=true.
	reciprocalLikeQuery = ""
	// $1 canonical person1 UUID, $2 person2 UUID. INSERT active match once.
	// Return id::text, person1_id::text, person2_id::text, status, created_at.
	// No conversation, deletion, reopening or overwriting of any existing pair.
	insertSwipeMatchQuery = ""
)

type SwipeRepository struct{ pool *pgxpool.Pool }

var _ domain.SwipeRepository = (*SwipeRepository)(nil)

func NewSwipeRepository(pool *pgxpool.Pool) *SwipeRepository { return &SwipeRepository{pool: pool} }

func canonicalRepositoryUUID(value string) (string, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != strings.ToLower(value) {
		return "", false
	}
	return parsed.String(), true
}

func (r *SwipeRepository) Save(ctx context.Context, fromUserID, toUserID string, isLike bool) (*domain.Match, error) {
	from, ok := canonicalRepositoryUUID(fromUserID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	to, ok := canonicalRepositoryUUID(toUserID)
	if !ok {
		return nil, domain.ErrInvalidUserID
	}
	if from == to {
		return nil, domain.ErrSelfSwipe
	}
	for _, query := range []string{searchStateByUserIDQuery, lockSwipeUsersQuery, lockSwipeProfilesQuery, lockSwipePreferencesQuery, lockSwipePhotosQuery,
		swipeHistoryConflictQuery, swipeMutualPreferencesQuery, insertSwipeQuery, reciprocalLikeQuery, insertSwipeMatchQuery} {
		if query == "" {
			return nil, fmt.Errorf("save swipe: %w", errQueryNotWritten)
		}
	}
	// Retry only known rolled-back PostgreSQL transactions, never ambiguous
	// network/commit errors. No token, partial swipe or match is returned on failure.
	for attempt := 0; attempt < 3; attempt++ {
		match, err := r.saveOnce(ctx, from, to, isLike)
		if err == nil {
			return match, nil
		}
		if ctx.Err() != nil || (pgErrorCode(err) != "40001" && pgErrorCode(err) != "40P01") || attempt == 2 {
			return nil, err
		}
	}
	panic("unreachable")
}

func (r *SwipeRepository) saveOnce(ctx context.Context, from, to string, isLike bool) (*domain.Match, error) {
	left, right := from, to
	if left > right {
		left, right = right, left
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("save swipe: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, query := range []string{lockSwipeUsersQuery, lockSwipeProfilesQuery, lockSwipePreferencesQuery, lockSwipePhotosQuery} {
		if _, err := tx.Exec(ctx, query, []string{left, right}); err != nil {
			return nil, fmt.Errorf("save swipe: lock resources: %w", err)
		}
	}
	state, err := readSearchState(ctx, tx, from)
	if err != nil {
		return nil, err
	}
	if err := state.CheckEligible(); err != nil {
		return nil, err
	}
	var conflict bool
	if err := tx.QueryRow(ctx, swipeHistoryConflictQuery, from, to).Scan(&conflict); err != nil {
		return nil, fmt.Errorf("save swipe: history: %w", err)
	}
	if conflict {
		return nil, domain.ErrConflict
	}
	state, err = readSearchState(ctx, tx, to)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrCandidateUnavailable
	}
	if err != nil {
		return nil, err
	}
	if state.CheckEligible() != nil {
		return nil, domain.ErrCandidateUnavailable
	}
	var mutual bool
	if err := tx.QueryRow(ctx, swipeMutualPreferencesQuery, from, to).Scan(&mutual); err != nil {
		return nil, fmt.Errorf("save swipe: preferences: %w", err)
	}
	if !mutual {
		return nil, domain.ErrCandidateUnavailable
	}
	if _, err := tx.Exec(ctx, insertSwipeQuery, from, to, isLike); err != nil {
		return nil, swipeWriteError("insert swipe", err)
	}
	var match *domain.Match
	if isLike {
		var reciprocal bool
		if err := tx.QueryRow(ctx, reciprocalLikeQuery, from, to).Scan(&reciprocal); err != nil {
			return nil, fmt.Errorf("save swipe: reciprocal like: %w", err)
		}
		if reciprocal {
			match = &domain.Match{}
			if err := tx.QueryRow(ctx, insertSwipeMatchQuery, left, right).Scan(&match.ID, &match.Person1ID, &match.Person2ID, &match.Status, &match.CreatedAt); err != nil {
				return nil, swipeWriteError("insert match", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("save swipe: commit: %w", err)
	}
	return match, nil
}

func swipeWriteError(operation string, err error) error {
	if pgErrorCode(err) == uniqueViolation {
		return fmt.Errorf("%s: %w", operation, domain.ErrConflict)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
