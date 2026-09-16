// Package chat describes conversations between matched users.
package chat

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type SendCommand struct {
	MatchID   string
	FromID    string
	Text      string
	ReplyToID *string
}

type Service interface {
	List(context.Context, string, string, int) ([]domain.Message, error)
	Send(context.Context, SendCommand) (domain.Message, error)
}
