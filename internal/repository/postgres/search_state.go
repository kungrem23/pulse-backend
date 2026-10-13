package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// $1 user UUID. Start from users, LEFT JOIN profile/preferences and
// count owned photos. Return exactly one row for an existing user, even before
// onboarding. Columns: is_blocked BOOL, has_completed_profile BOOL,
// has_valid_preferences BOOL, photo_count INT, is_sleeping BOOL (false if no
// profile). No NULL values. No user -> zero rows, mapped to ErrNotFound.
// Profile completeness includes name, age (users), sex and city, not just its
// existence. Preferences must have the agreed age range and sex/any values.
const searchStateByUserIDQuery = `
SELECT u.is_blocked,
       COALESCE(char_length(btrim(p.name)) BETWEEN 1 AND 100
           AND u.age BETWEEN 18 AND 120 AND p.sex IN ('male', 'female')
           AND char_length(btrim(p.city)) BETWEEN 1 AND 100, false),
       COALESCE(pr.min_age BETWEEN 18 AND 120 AND pr.max_age BETWEEN 18 AND 120
           AND pr.min_age <= pr.max_age AND pr.sex IN ('male', 'female', 'any'), false),
       (SELECT count(*)::int FROM photos ph WHERE ph.user_id = u.id),
       COALESCE(p.is_sleeping, false)
FROM users u
LEFT JOIN profiles p ON p.user_id = u.id
LEFT JOIN preferences pr ON pr.user_id = p.user_id
WHERE u.id = $1::uuid
`

type SearchStateRepository struct {
	pool *pgxpool.Pool
}

var _ domain.SearchStateRepository = (*SearchStateRepository)(nil)

func NewSearchStateRepository(pool *pgxpool.Pool) *SearchStateRepository {
	return &SearchStateRepository{pool: pool}
}

func (r *SearchStateRepository) StateByUserID(ctx context.Context, userID string) (domain.SearchState, error) {
	return readSearchState(ctx, r.pool, userID)
}

type searchStateQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// The discovery transaction uses the same query on its own snapshot. Matching
// may use StateByUserID for a precheck but must recheck in the swipe transaction.
func readSearchState(ctx context.Context, queries searchStateQuerier, userID string) (domain.SearchState, error) {
	var state domain.SearchState
	err := queries.QueryRow(ctx, searchStateByUserIDQuery, userID).Scan(
		&state.IsBlocked, &state.HasProfile, &state.HasPreferences, &state.PhotoCount, &state.IsSleeping,
	)
	if err != nil {
		return domain.SearchState{}, notFoundOr("get search state", err)
	}
	return state, nil
}
