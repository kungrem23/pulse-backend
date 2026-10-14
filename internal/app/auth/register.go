package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
)

const (
	minimumPasswordCharacters  = 8
	maximumBcryptPasswordBytes = 72
	maximumEmailBytes          = 254
)

func (s *service) Register(ctx context.Context, command RegisterCommand) (Registration, error) {
	email, err := normalizeEmail(command.Email)
	if err != nil {
		return Registration{}, ErrInvalidEmail
	}
	if err := validatePassword(command.Password); err != nil {
		return Registration{}, err
	}

	passwordHash, err := s.passwords.Hash(command.Password)
	if err != nil {
		return Registration{}, fmt.Errorf("hash user password: %w", err)
	}

	user, err := s.users.Create(ctx, domain.User{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return Registration{}, ErrEmailAlreadyExists
		}
		return Registration{}, fmt.Errorf("create user: %w", err)
	}

	session, err := s.startSession(ctx, user.ID)
	if err != nil {
		return Registration{}, err
	}

	user.PasswordHash = ""
	return Registration{
		User:    user,
		Session: session,
	}, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" || len([]byte(email)) > maximumEmailBytes {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minimumPasswordCharacters ||
		len([]byte(password)) > maximumBcryptPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

func sessionFromTokenPair(pair security.TokenPair) Session {
	return Session{
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt,
		RefreshExpiresAt: pair.RefreshExpiresAt,
	}
}
