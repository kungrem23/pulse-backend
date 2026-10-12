// Package media describes profile-photo use cases.
package media

import (
	"context"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type UploadCommand struct {
	UserID      string
	ContentType string
	Content     []byte
	Position    int16
}

type Service interface {
	Upload(context.Context, UploadCommand) (domain.Photo, error)
	Delete(context.Context, string, string) error
}
