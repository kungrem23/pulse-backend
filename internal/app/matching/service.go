// Package matching describes swipe and match use cases.
package matching

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type SwipeCommand struct {
	FromUserID string
	ToUserID   string
	IsLike     bool
}

type Service interface {
	Swipe(context.Context, SwipeCommand) (*domain.Match, error)
	List(context.Context, string, int) ([]domain.Match, error)
}
