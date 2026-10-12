// Package moderation describes reports and administrator use cases.
package moderation

import (
	"context"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type CreateReportCommand struct {
	FromUserID string
	ToUserID   string
	Reason     string
	Comment    string
}

type ProcessReportCommand struct {
	ReportID   string
	AdminID    string
	Status     string
	Resolution string
	Comment    string
}

type Service interface {
	CreateReport(context.Context, CreateReportCommand) error
	ListReports(context.Context, string, domain.PageRequest) (domain.Page[domain.Report], error)
	ProcessReport(context.Context, ProcessReportCommand) error
	InspectUser(context.Context, string) (domain.User, error)
	BlockUser(context.Context, string, string) error
	MessageUser(context.Context, string, string, string) error
	NotifyUser(context.Context, string, string, string) error
}
