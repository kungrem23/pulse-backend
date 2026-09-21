package adminauth

import (
	"context"

	"github.com/example/pulse-backend/internal/domain"
)

type LoginCommand struct {
	Username string
	Password string
}

type Session struct {
	AccessToken  string
	RefreshToken string
}

type Service interface {
	Login(context.Context, LoginCommand) (Session, error)
}

type service struct {
	admins domain.AdminRepository
}

func NewService(admins domain.AdminRepository) Service {
	return &service{admins: admins}
}
