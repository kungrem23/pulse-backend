package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

func (s *service) Delete(ctx context.Context, userID, photoID string) error {
	if !validUUID(userID) {
		return ErrInvalidUserID
	}
	if !validUUID(photoID) {
		return ErrInvalidPhotoID
	}

	photo, err := s.photos.ByIDForUser(ctx, photoID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrPhotoNotFound
		}
		return fmt.Errorf("find owned photo: %w", err)
	}

	// S3 deletion is idempotent. If the following metadata deletion fails, a
	// repeated request safely retries both steps and heals the partial state.
	if err := s.storage.Delete(ctx, photo.ObjectKey); err != nil {
		return fmt.Errorf("delete photo object: %w", err)
	}
	if err := s.photos.Delete(ctx, photoID, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrPhotoNotFound
		}
		return fmt.Errorf("delete photo metadata: %w", err)
	}
	return nil
}
