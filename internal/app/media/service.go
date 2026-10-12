// Package media describes profile-photo use cases.
package media

import (
	"context"
	"errors"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

var (
	ErrInvalidUserID    = errors.New("invalid user ID")
	ErrInvalidPhotoID   = errors.New("invalid photo ID")
	ErrInvalidPosition  = errors.New("photo position must be between 1 and 5")
	ErrPhotoTooLarge    = errors.New("photo exceeds 10 MiB")
	ErrUnsupportedPhoto = errors.New("photo must contain a valid JPEG, PNG, or WebP image")
	ErrPositionOccupied = errors.New("photo position is occupied")
	ErrPhotoNotFound    = errors.New("photo not found")
)

type UploadCommand struct {
	UserID   string
	Content  []byte
	Position int16
}

// Photo is the application result safe for transport. It deliberately omits
// the owner and the private object key stored by the domain entity.
type Photo struct {
	ID       string
	URL      string
	Position int16
}

type Service interface {
	Upload(context.Context, UploadCommand) (Photo, error)
	Delete(context.Context, string, string) error
}

type service struct {
	photos  domain.PhotoRepository
	storage domain.PhotoStorage
	newID   func() string
}

func NewService(photos domain.PhotoRepository, storage domain.PhotoStorage) Service {
	return newService(photos, storage, newUUID)
}

func newService(photos domain.PhotoRepository, storage domain.PhotoStorage, newID func() string) *service {
	return &service{photos: photos, storage: storage, newID: newID}
}
