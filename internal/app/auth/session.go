package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
)

func (s *service) startSession(ctx context.Context, userID string) (Session, error) {
	pair, err := s.tokens.Issue(userID, security.UserPrincipal)
	if err != nil {
		return Session{}, fmt.Errorf("issue user session: %w", err)
	}
	err = s.sessions.Create(ctx, domain.RefreshSession{
		UserID:    userID,
		TokenHash: hashRefreshToken(pair.RefreshToken),
		ExpiresAt: pair.RefreshExpiresAt,
	})
	if err != nil {
		return Session{}, fmt.Errorf("store refresh session: %w", err)
	}
	return sessionFromTokenPair(pair), nil
}

func (s *service) Refresh(ctx context.Context, command RefreshCommand) (Session, error) {
	claims, err := s.verifyUserRefreshToken(command.RefreshToken)
	if err != nil {
		return Session{}, err
	}

	// The account state is checked again: a valid refresh JWT may outlive a block.
	user, err := s.users.ByID(ctx, claims.Subject)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Session{}, ErrInvalidRefreshToken
		}
		return Session{}, fmt.Errorf("find refresh session user: %w", err)
	}
	if user.IsBlocked {
		return Session{}, ErrUserBlocked
	}

	pair, err := s.tokens.Issue(user.ID, security.UserPrincipal)
	if err != nil {
		return Session{}, fmt.Errorf("issue user session: %w", err)
	}
	err = s.sessions.Rotate(ctx, hashRefreshToken(command.RefreshToken), domain.RefreshSession{
		UserID:    user.ID,
		TokenHash: hashRefreshToken(pair.RefreshToken),
		ExpiresAt: pair.RefreshExpiresAt,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Session{}, ErrInvalidRefreshToken
		}
		return Session{}, fmt.Errorf("rotate refresh session: %w", err)
	}
	return sessionFromTokenPair(pair), nil
}

func (s *service) Logout(ctx context.Context, command LogoutCommand) error {
	claims, err := s.verifyUserRefreshToken(command.RefreshToken)
	if err != nil {
		return err
	}
	if command.UserID == "" || claims.Subject != command.UserID {
		return ErrInvalidRefreshToken
	}
	if err := s.sessions.Revoke(ctx, command.UserID, hashRefreshToken(command.RefreshToken)); err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func (s *service) verifyUserRefreshToken(rawToken string) (security.Claims, error) {
	claims, err := s.tokens.Verify(rawToken, security.RefreshToken)
	if err != nil || claims.Principal != security.UserPrincipal {
		return security.Claims{}, ErrInvalidRefreshToken
	}
	return claims, nil
}

// Refresh JWTs carry 128+ bits of randomness (jti), so a fast unsalted hash is
// sufficient; bcrypt would only add latency to every refresh.
func hashRefreshToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
