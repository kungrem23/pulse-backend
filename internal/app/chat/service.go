// Package chat describes conversations between matched users.
package chat

import (
	"context"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type SendCommand struct {
	ConversationID string
	FromID         string
	Text           string
	ReplyToID      *string
}

type Service interface {
	Conversations(context.Context, string, domain.PageRequest) (domain.Page[domain.Conversation], error)
	List(context.Context, string, string, domain.PageRequest) (domain.Page[domain.Message], error)
	Send(context.Context, SendCommand) (domain.Message, error)
}
