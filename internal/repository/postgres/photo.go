package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// Parameter and scanned column order follow the repository contract.
// decisions_002 constraints protect positions 1–5 and UNIQUE(user_id, position),
// including concurrent uploads. Do not upsert
// or overwrite another photo when a position is occupied.
const (
	// Params: $1 photo UUID, $2 owner UUID, $3 private object_key, $4 position.
	// Inserts exactly one photos row; the profiles FK must be enforced.
	// Duplicate position -> 23505; absent owner profile -> 23503.
	createPhotoQuery = `
INSERT INTO photos (id, user_id, object_key, position)
VALUES ($1::uuid, $2::uuid, $3, $4)
`

	// Params: $1 photo UUID, $2 owner UUID. BOTH must be in the WHERE clause.
	// Returns zero or one row: id::text, user_id::text, object_key, position.
	photoByIDForUserQuery = `
SELECT id::text, user_id::text, object_key, position FROM photos
WHERE id = $1::uuid AND user_id = $2::uuid
`

	// Params: $1 owner UUID. Returns only this owner's photos ordered position ASC.
	// Columns: id::text, user_id::text, object_key, position.
	photosByUserIDQuery = `
SELECT id::text, user_id::text, object_key, position FROM photos
WHERE user_id = $1::uuid ORDER BY position ASC
`

	// Params: $1 photo UUID, $2 owner UUID. BOTH must be in the WHERE clause.
	// Deletes metadata only; no changes to profile/preferences or S3.
	// Zero affected rows means missing/foreign photo.
	deletePhotoQuery = `
DELETE FROM photos WHERE id = $1::uuid AND user_id = $2::uuid
`
)

type PhotoRepository struct {
	pool *pgxpool.Pool
}

var _ domain.PhotoRepository = (*PhotoRepository)(nil)

func NewPhotoRepository(pool *pgxpool.Pool) *PhotoRepository {
	return &PhotoRepository{pool: pool}
}

func (r *PhotoRepository) Create(ctx context.Context, photo domain.Photo) error {
	tag, err := r.pool.Exec(ctx, createPhotoQuery, photo.ID, photo.UserID, photo.ObjectKey, photo.Position)
	if err != nil {
		switch pgErrorCode(err) {
		case uniqueViolation:
			return fmt.Errorf("create photo: %w", domain.ErrConflict)
		case foreignKeyViolation:
			return fmt.Errorf("create photo: %w", domain.ErrNotFound)
		default:
			return fmt.Errorf("create photo: %w", err)
		}
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("create photo: %w", domain.ErrNotFound)
	}
	return nil
}

func (r *PhotoRepository) ByIDForUser(ctx context.Context, photoID, userID string) (domain.Photo, error) {
	var photo domain.Photo
	err := r.pool.QueryRow(ctx, photoByIDForUserQuery, photoID, userID).Scan(
		&photo.ID, &photo.UserID, &photo.ObjectKey, &photo.Position,
	)
	if err != nil {
		return domain.Photo{}, notFoundOr("find owned photo", err)
	}
	return photo, nil
}

func (r *PhotoRepository) ByUserID(ctx context.Context, userID string) ([]domain.Photo, error) {
	rows, err := r.pool.Query(ctx, photosByUserIDQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("list photos: %w", err)
	}
	defer rows.Close()
	photos := make([]domain.Photo, 0, domain.MaxPhotos)
	for rows.Next() {
		var photo domain.Photo
		if err := rows.Scan(&photo.ID, &photo.UserID, &photo.ObjectKey, &photo.Position); err != nil {
			return nil, fmt.Errorf("list photos: scan: %w", err)
		}
		photos = append(photos, photo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list photos: %w", err)
	}
	return photos, nil
}

func (r *PhotoRepository) Delete(ctx context.Context, photoID, userID string) error {
	tag, err := r.pool.Exec(ctx, deletePhotoQuery, photoID, userID)
	if err != nil {
		return notFoundOr("delete photo", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete photo: %w", domain.ErrNotFound)
	}
	return nil
}
