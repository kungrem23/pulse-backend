package adminauth

import (
	"context"
	"errors"
	"time"

	"github.com/example/pulse-backend/internal/domain"
	"github.com/example/pulse-backend/internal/security"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type LoginCommand struct {
	Username string
	Password string
}

type Session struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Service interface {
	Login(context.Context, LoginCommand) (Session, error)
}

type PasswordVerifier interface {
	Verify(password, encodedHash string) error
}

type TokenIssuer interface {
	Issue(subject string, principal security.PrincipalType) (security.TokenPair, error)
}

type service struct {
	admins    domain.AdminRepository
	passwords PasswordVerifier
	tokens    TokenIssuer
}

func NewService(admins domain.AdminRepository, passwords PasswordVerifier, tokens TokenIssuer) Service {
	return &service{
		admins:    admins,
		passwords: passwords,
		tokens:    tokens,
	}
}
