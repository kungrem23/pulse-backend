// Package moderation describes reports and administrator use cases.
package moderation

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type CreateReportCommand struct {
	FromUserID string
	ToUserID   string
	Reason     string
}

type Service interface {
	CreateReport(context.Context, CreateReportCommand) error
	PendingReports(context.Context, int) ([]domain.Report, error)
	ProcessReport(context.Context, string, string) error
	InspectUser(context.Context, string) (domain.User, error)
	BlockUser(context.Context, string, string) error
	MessageUser(context.Context, string, string, string) error
	NotifyUser(context.Context, string, string, string) error
}
