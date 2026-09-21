package adminauth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/example/pulse-backend/internal/domain"
	"github.com/example/pulse-backend/internal/security"
)

// dummyAdminPasswordHash has the same bcrypt work factor as production hashes.
// It is checked when a username is unknown to reduce username-enumeration timing
// differences. Its plaintext value is irrelevant and it grants no access.
const dummyAdminPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *service) Login(ctx context.Context, command LoginCommand) (Session, error) {
	username := strings.TrimSpace(command.Username)
	if username == "" || command.Password == "" {
		return Session{}, ErrInvalidCredentials
	}

	admin, err := s.admins.ByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Ignore the result: this comparison only makes the not-found path do
			// approximately the same amount of work as a regular failed login.
			_ = s.passwords.Verify(command.Password, dummyAdminPasswordHash)
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("find admin by username: %w", err)
	}

	if err := s.passwords.Verify(command.Password, admin.PasswordHash); err != nil {
		if errors.Is(err, security.ErrPasswordMismatch) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("verify admin password: %w", err)
	}

	tokenPair, err := s.tokens.Issue(admin.ID, security.AdminPrincipal)
	if err != nil {
		return Session{}, fmt.Errorf("issue admin session: %w", err)
	}

	return Session{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		AccessExpiresAt:  tokenPair.AccessExpiresAt,
		RefreshExpiresAt: tokenPair.RefreshExpiresAt,
	}, nil
}
