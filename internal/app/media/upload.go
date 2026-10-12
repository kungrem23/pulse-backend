package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

const compensationTimeout = 5 * time.Second

func (s *service) Upload(ctx context.Context, command UploadCommand) (Photo, error) {
	if !validUUID(command.UserID) {
		return Photo{}, ErrInvalidUserID
	}
	if command.Position < 1 || command.Position > domain.MaxPhotos {
		return Photo{}, ErrInvalidPosition
	}
	format, err := inspectImage(command.Content)
	if err != nil {
		return Photo{}, err
	}

	photoID := s.newID()
	if !validUUID(photoID) {
		return Photo{}, fmt.Errorf("generate photo ID: invalid UUID")
	}
	objectKey := fmt.Sprintf("users/%s/photos/%s%s", command.UserID, photoID, format.extension)
	storedKey, err := s.storage.Put(ctx, objectKey, command.Content, format.contentType)
	if err != nil {
		return Photo{}, fmt.Errorf("store photo: %w", err)
	}

	photoURL, err := s.storage.URL(ctx, storedKey)
	if err != nil {
		return Photo{}, s.compensate(ctx, storedKey, fmt.Errorf("create photo URL: %w", err))
	}

	entity := domain.Photo{
		ID:        photoID,
		UserID:    command.UserID,
		ObjectKey: storedKey,
		Position:  command.Position,
	}
	if err := s.photos.Create(ctx, entity); err != nil {
		cause := fmt.Errorf("create photo metadata: %w", err)
		if errors.Is(err, domain.ErrConflict) {
			cause = ErrPositionOccupied
		}
		return Photo{}, s.compensate(ctx, storedKey, cause)
	}

	return Photo{ID: photoID, URL: photoURL, Position: command.Position}, nil
}

func (s *service) compensate(ctx context.Context, objectKey string, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationTimeout)
	defer cancel()
	if err := s.storage.Delete(cleanupCtx, objectKey); err != nil {
		return errors.Join(cause, fmt.Errorf("compensate photo object: %w", err))
	}
	return cause
}
