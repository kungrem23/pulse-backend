package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// Parameter and result column order follow the repository contract.
const (
	// Params: $1 caller UUID, $2 after-created_at (nullable), $3 after-user UUID
	// (nullable), $4 requested limit + 1 for lookahead. Never LIMIT joined photos.
	// Columns: candidate user_id::text, name, bio, users.age, sex, city,
	// users.created_at. One row per candidate, ordered created_at DESC, id DESC.
	// Exclusive keyset boundary is (users.created_at, users.id) < ($2, $3).
	// All filtering MUST happen before LIMIT, using both profiles/preferences:
	// 1) candidate age in caller range; 2) candidate sex matches caller or any;
	// 3) caller age in candidate range; 4) caller sex matches candidate or any.
	// No city filter. Exclude self, blocked/sleeping/incomplete users, missing
	// preferences/photos, any outgoing swipe (like or pass), and ANY existing
	// match with this caller in either pair position, including closed matches.
	// Incoming swipes alone do not exclude a candidate. Do not reset history.
	discoveryCandidatesQuery = `
SELECT u.id::text, p.name, p.bio, u.age, p.sex, p.city, u.created_at
FROM users me
JOIN profiles mp ON mp.user_id = me.id
JOIN preferences mine ON mine.user_id = me.id
JOIN users u ON u.id <> me.id
JOIN profiles p ON p.user_id = u.id
JOIN preferences theirs ON theirs.user_id = u.id
WHERE me.id = $1::uuid
  AND NOT me.is_blocked AND NOT mp.is_sleeping
  AND NOT u.is_blocked AND NOT p.is_sleeping
  AND u.age BETWEEN mine.min_age AND mine.max_age
  AND (mine.sex = 'any' OR mine.sex = p.sex)
  AND me.age BETWEEN theirs.min_age AND theirs.max_age
  AND (theirs.sex = 'any' OR theirs.sex = mp.sex)
  AND EXISTS (SELECT 1 FROM photos ph WHERE ph.user_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM swipes s WHERE s.from_user_id = me.id AND s.to_user_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM matches m
      WHERE m.person1_id = LEAST(me.id, u.id) AND m.person2_id = GREATEST(me.id, u.id))
  AND ($2::timestamptz IS NULL OR (u.created_at, u.id) < ($2::timestamptz, $3::uuid))
ORDER BY u.created_at DESC, u.id DESC
LIMIT $4
`

	// Params: $1 array of the returned candidate user UUIDs, NOT caller UUID.
	// Return their photos in one query, ordered user_id, position ASC.
	// Columns: id::text, user_id::text, object_key, position. No URL in the DB.
	// Restrict rows to that UUID array; do not load photos of unrelated users.
	discoveryPhotosQuery = `
SELECT id::text, user_id::text, object_key, position
FROM photos WHERE user_id = ANY($1::uuid[])
ORDER BY user_id, position ASC
`
)

type DiscoveryRepository struct {
	pool *pgxpool.Pool
}

var _ domain.DiscoveryRepository = (*DiscoveryRepository)(nil)

func NewDiscoveryRepository(pool *pgxpool.Pool) *DiscoveryRepository {
	return &DiscoveryRepository{pool: pool}
}

func (r *DiscoveryRepository) Feed(ctx context.Context, userID string, page domain.PageQuery) (domain.PageResult[domain.DiscoveryCandidate], error) {
	if page.Limit < 1 || page.Limit > 100 {
		return domain.PageResult[domain.DiscoveryCandidate]{}, domain.ErrInvalidLimit
	}
	// Recheck the caller and load candidates/photos on one snapshot, so a
	// concurrent last-photo deletion cannot produce a half-populated card.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := readSearchState(ctx, tx, userID)
	if err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, err
	}
	if err := state.CheckEligible(); err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, err
	}
	var afterTime, afterID any
	if page.After != nil {
		afterTime, afterID = page.After.CreatedAt.UTC(), page.After.ID
	}
	rows, err := tx.Query(ctx, discoveryCandidatesQuery, userID, afterTime, afterID, page.Limit+1)
	if err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: candidates: %w", err)
	}
	candidates := make([]domain.DiscoveryCandidate, 0, page.Limit+1)
	for rows.Next() {
		var candidate domain.DiscoveryCandidate
		var sex string
		err := rows.Scan(&candidate.Profile.UserID, &candidate.Profile.Name, &candidate.Profile.Bio,
			&candidate.Profile.Age, &sex, &candidate.Profile.City, &candidate.CreatedAt)
		if err != nil {
			rows.Close()
			return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: scan candidate: %w", err)
		}
		candidate.Profile.Sex = domain.Sex(sex)
		candidate.Photos = make([]domain.Photo, 0, domain.MaxPhotos)
		candidates = append(candidates, candidate)
		if len(candidates) > page.Limit+1 {
			rows.Close()
			return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: query exceeded lookahead limit")
		}
	}
	err = rows.Err()
	rows.Close() // Release the connection's result set before the bulk-photo query.
	if err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: read candidates: %w", err)
	}
	result := domain.PageResult[domain.DiscoveryCandidate]{Items: candidates, HasMore: len(candidates) > page.Limit}
	if result.HasMore {
		result.Items = result.Items[:page.Limit]
	}
	if len(result.Items) > 0 {
		if err := loadDiscoveryPhotos(ctx, tx, result.Items); err != nil {
			return domain.PageResult[domain.DiscoveryCandidate]{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PageResult[domain.DiscoveryCandidate]{}, fmt.Errorf("get feed: commit: %w", err)
	}
	return result, nil
}

func loadDiscoveryPhotos(ctx context.Context, tx pgx.Tx, candidates []domain.DiscoveryCandidate) error {
	ids := make([]string, 0, len(candidates))
	byUser := make(map[string]*domain.DiscoveryCandidate, len(candidates))
	for i := range candidates {
		candidate := &candidates[i]
		ids = append(ids, candidate.Profile.UserID)
		byUser[candidate.Profile.UserID] = candidate
	}
	rows, err := tx.Query(ctx, discoveryPhotosQuery, ids)
	if err != nil {
		return fmt.Errorf("get feed: photos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var photo domain.Photo
		if err := rows.Scan(&photo.ID, &photo.UserID, &photo.ObjectKey, &photo.Position); err != nil {
			return fmt.Errorf("get feed: scan photo: %w", err)
		}
		candidate, ok := byUser[photo.UserID]
		if !ok || photo.Position < 1 || photo.Position > domain.MaxPhotos {
			return fmt.Errorf("get feed: unexpected photo owner or position")
		}
		if count := len(candidate.Photos); count > 0 && candidate.Photos[count-1].Position >= photo.Position {
			return fmt.Errorf("get feed: photo positions are duplicated or unordered")
		}
		candidate.Photos = append(candidate.Photos, photo)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("get feed: read photos: %w", err)
	}
	for _, candidate := range candidates {
		if len(candidate.Photos) == 0 {
			return fmt.Errorf("get feed: eligible candidate has no photos on the snapshot")
		}
	}
	return nil
}
