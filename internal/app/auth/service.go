// Package auth describes registration and authentication use cases.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidAge         = errors.New("age must be between 18 and 120")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrUserBlocked        = errors.New("user is blocked")
)

type RegisterCommand struct {
	Email    string
	Password string
	Age      int
}

type LoginCommand struct {
	Email    string
	Password string
}

type Session struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Registration struct {
	User    domain.User
	Session Session
}

type Service interface {
	Register(context.Context, RegisterCommand) (Registration, error)
	Login(context.Context, LoginCommand) (Session, error)
}

type PasswordManager interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) error
}

type TokenIssuer interface {
	Issue(subject string, principal security.PrincipalType) (security.TokenPair, error)
}

type service struct {
	users     domain.UserRepository
	passwords PasswordManager
	tokens    TokenIssuer
}

func NewService(users domain.UserRepository, passwords PasswordManager, tokens TokenIssuer) Service {
	return &service{
		users:     users,
		passwords: passwords,
		tokens:    tokens,
	}
}
