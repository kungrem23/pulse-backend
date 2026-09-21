package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/pulse-backend/internal/domain"
	"github.com/example/pulse-backend/internal/security"
)

// dummyUserPasswordHash is checked for unknown emails so that a failed login
// performs approximately the same bcrypt work whether or not the user exists.
const dummyUserPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *service) Login(ctx context.Context, command LoginCommand) (Session, error) {
	email, err := normalizeEmail(command.Email)
	if err != nil || command.Password == "" {
		return Session{}, ErrInvalidCredentials
	}

	user, err := s.users.ByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			_ = s.passwords.Verify(command.Password, dummyUserPasswordHash)
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("find user by email: %w", err)
	}

	if err := s.passwords.Verify(command.Password, user.PasswordHash); err != nil {
		if errors.Is(err, security.ErrPasswordMismatch) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("verify user password: %w", err)
	}
	if user.IsBlocked {
		return Session{}, ErrUserBlocked
	}

	tokenPair, err := s.tokens.Issue(user.ID, security.UserPrincipal)
	if err != nil {
		return Session{}, fmt.Errorf("issue user session: %w", err)
	}

	return sessionFromTokenPair(tokenPair), nil
}
