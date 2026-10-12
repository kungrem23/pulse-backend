// Package matching describes swipe and match use cases.
package matching

import (
	"context"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type SwipeCommand struct {
	FromUserID string
	ToUserID   string
	IsLike     bool
}

type Service interface {
	Swipe(context.Context, SwipeCommand) (*domain.Match, error)
	List(context.Context, string, domain.PageRequest) (domain.Page[domain.Match], error)
	// Close is post-MVP. The pair remains permanently excluded from discovery.
	Close(context.Context, string, string) error
}
