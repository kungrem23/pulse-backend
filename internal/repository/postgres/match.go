package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// $1 caller UUID, $2 after-created_at (nullable), $3 after-match UUID
// (nullable), $4 limit+1. Only matches where caller is person1 OR person2.
// Order matches.created_at DESC, matches.id DESC; exclusive keyset boundary.
// JOIN the OTHER partner's profile and users.age, including closed pairs and
// sleeping/blocked/incomplete-discovery partners. No discovery filters.
// Load the partner's lowest-position photo with one join/lateral subquery,
// not one query per pair; optional photo columns below must ALL be NULL if absent.
// Columns: match id::text, person1_id::text, person2_id::text, status, created_at;
// partner user_id::text, name, bio, users.age, sex, city, is_sleeping;
// optional photo id::text, user_id::text, object_key, position.
// LIMIT match rows, not one-to-many photo rows. Do not require a conversation.
const matchesByUserIDQuery = `
SELECT m.id::text, m.person1_id::text, m.person2_id::text, m.status, m.created_at,
       p.user_id::text, p.name, p.bio, u.age, p.sex, p.city, p.is_sleeping,
       ph.id::text, ph.user_id::text, ph.object_key, ph.position
FROM matches m
JOIN profiles p ON p.user_id = CASE WHEN m.person1_id = $1::uuid THEN m.person2_id ELSE m.person1_id END
JOIN users u ON u.id = p.user_id
LEFT JOIN LATERAL (
    SELECT id, user_id, object_key, position FROM photos
    WHERE user_id = p.user_id ORDER BY position ASC LIMIT 1
) ph ON true
WHERE (m.person1_id = $1::uuid OR m.person2_id = $1::uuid)
  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) < ($2::timestamptz, $3::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4
`

type MatchRepository struct{ pool *pgxpool.Pool }

var _ domain.MatchRepository = (*MatchRepository)(nil)

func NewMatchRepository(pool *pgxpool.Pool) *MatchRepository { return &MatchRepository{pool: pool} }

func (r *MatchRepository) ListByUserID(ctx context.Context, userID string, page domain.PageQuery) (domain.PageResult[domain.MatchWithPartner], error) {
	owner, ok := canonicalRepositoryUUID(userID)
	if !ok {
		return domain.PageResult[domain.MatchWithPartner]{}, domain.ErrNotFound
	}
	if page.Limit < 1 || page.Limit > 100 {
		return domain.PageResult[domain.MatchWithPartner]{}, domain.ErrInvalidLimit
	}
	var afterTime, afterID any
	if page.After != nil {
		afterTime, afterID = page.After.CreatedAt.UTC(), page.After.ID
	}
	rows, err := r.pool.Query(ctx, matchesByUserIDQuery, owner, afterTime, afterID, page.Limit+1)
	if err != nil {
		return domain.PageResult[domain.MatchWithPartner]{}, fmt.Errorf("list matches: %w", err)
	}
	defer rows.Close()
	items := make([]domain.MatchWithPartner, 0, page.Limit+1)
	for rows.Next() {
		var entry domain.MatchWithPartner
		var sex string
		var photoID, photoOwner, key *string
		var position *int16
		err := rows.Scan(&entry.Match.ID, &entry.Match.Person1ID, &entry.Match.Person2ID, &entry.Match.Status, &entry.Match.CreatedAt,
			&entry.Partner.UserID, &entry.Partner.Name, &entry.Partner.Bio, &entry.Partner.Age, &sex, &entry.Partner.City, &entry.Partner.IsSleeping,
			&photoID, &photoOwner, &key, &position)
		if err != nil {
			return domain.PageResult[domain.MatchWithPartner]{}, fmt.Errorf("list matches: scan: %w", err)
		}
		entry.Partner.Sex = domain.Sex(sex)
		if photoID != nil || photoOwner != nil || key != nil || position != nil {
			if photoID == nil || photoOwner == nil || key == nil || position == nil {
				return domain.PageResult[domain.MatchWithPartner]{}, fmt.Errorf("list matches: incomplete photo projection")
			}
			entry.Photo = &domain.Photo{ID: *photoID, UserID: *photoOwner, ObjectKey: *key, Position: *position}
		}
		items = append(items, entry)
		if len(items) > page.Limit+1 {
			return domain.PageResult[domain.MatchWithPartner]{}, fmt.Errorf("list matches: query exceeded lookahead limit")
		}
	}
	if err := rows.Err(); err != nil {
		return domain.PageResult[domain.MatchWithPartner]{}, fmt.Errorf("list matches: read: %w", err)
	}
	result := domain.PageResult[domain.MatchWithPartner]{Items: items, HasMore: len(items) > page.Limit}
	if result.HasMore {
		result.Items = result.Items[:page.Limit]
	}
	return result, nil
}
