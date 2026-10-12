package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
)

const sessionUserID = "6130ce38-a239-447c-b65a-a2019b370962"

func newSessionTestService(t *testing.T, user domain.User) (Service, *sessionRepositoryStub, *security.TokenIssuer) {
	t.Helper()
	sessions := &sessionRepositoryStub{}
	tokens := newTestTokenIssuer(t)
	users := &userRepositoryStub{byIDUser: user, byEmailUser: user}
	return NewService(users, sessions, &passwordManagerStub{}, tokens), sessions, tokens
}

func issueRefreshToken(t *testing.T, tokens *security.TokenIssuer, subject string, principal security.PrincipalType) security.TokenPair {
	t.Helper()
	pair, err := tokens.Issue(subject, principal)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func TestLoginStoresOnlyRefreshTokenHash(t *testing.T) {
	service, sessions, _ := newSessionTestService(t, domain.User{ID: sessionUserID, PasswordHash: "hash"})

	session, err := service.Login(context.Background(), LoginCommand{Email: "user@example.com", Password: "password"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(sessions.created) != 1 {
		t.Fatalf("expected one stored session, got %d", len(sessions.created))
	}
	stored := sessions.created[0]
	if stored.UserID != sessionUserID || !stored.ExpiresAt.Equal(session.RefreshExpiresAt) {
		t.Fatalf("unexpected stored session: %+v", stored)
	}
	if stored.TokenHash == session.RefreshToken || stored.TokenHash != hashRefreshToken(session.RefreshToken) {
		t.Fatal("repository must receive the refresh token hash, not the raw token")
	}
}

func TestLoginFailsWhenSessionCannotBeStored(t *testing.T) {
	service, sessions, _ := newSessionTestService(t, domain.User{ID: sessionUserID, PasswordHash: "hash"})
	sessions.createErr = errors.New("database unavailable")

	if _, err := service.Login(context.Background(), LoginCommand{Email: "user@example.com", Password: "password"}); err == nil {
		t.Fatal("tokens must not be returned without a stored session")
	}
}

func TestRefreshRotatesSession(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID})
	old := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)

	session, err := service.Refresh(context.Background(), RefreshCommand{RefreshToken: old.RefreshToken})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if session.RefreshToken == old.RefreshToken {
		t.Fatal("refresh must issue a new refresh token")
	}
	if sessions.rotatedHash != hashRefreshToken(old.RefreshToken) ||
		sessions.rotatedNext.TokenHash != hashRefreshToken(session.RefreshToken) ||
		sessions.rotatedNext.UserID != sessionUserID ||
		!sessions.rotatedNext.ExpiresAt.Equal(session.RefreshExpiresAt) {
		t.Fatalf("unexpected rotation: old=%q next=%+v", sessions.rotatedHash, sessions.rotatedNext)
	}
	if claims, err := tokens.Verify(session.AccessToken, security.AccessToken); err != nil || claims.Subject != sessionUserID {
		t.Fatalf("new access token is invalid: %+v, %v", claims, err)
	}
}

func TestRefreshRejectsUnusableTokensWithoutRotation(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID})
	userPair := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)
	adminPair := issueRefreshToken(t, tokens, sessionUserID, security.AdminPrincipal)
	unknownUser := issueRefreshToken(t, tokens, "00000000-0000-0000-0000-000000000000", security.UserPrincipal)

	for name, token := range map[string]string{
		"garbage":       "not-a-jwt",
		"access token":  userPair.AccessToken,
		"admin refresh": adminPair.RefreshToken,
		"unknown user":  unknownUser.RefreshToken,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Refresh(context.Background(), RefreshCommand{RefreshToken: token}); !errors.Is(err, ErrInvalidRefreshToken) {
				t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
			}
		})
	}
	if sessions.rotateCalls != 0 {
		t.Fatalf("invalid tokens must not reach the repository, got %d rotations", sessions.rotateCalls)
	}
}

func TestRefreshMapsMissingSessionToInvalidToken(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID})
	sessions.rotateErr = domain.ErrNotFound
	pair := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)

	if _, err := service.Refresh(context.Background(), RefreshCommand{RefreshToken: pair.RefreshToken}); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for revoked/reused session, got %v", err)
	}
}

func TestRefreshRejectsBlockedUser(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID, IsBlocked: true})
	pair := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)

	if _, err := service.Refresh(context.Background(), RefreshCommand{RefreshToken: pair.RefreshToken}); !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("expected ErrUserBlocked, got %v", err)
	}
	if sessions.rotateCalls != 0 {
		t.Fatal("a blocked user must not get a rotated session")
	}
}

func TestLogoutRevokesOwnSession(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID})
	pair := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)

	if err := service.Logout(context.Background(), LogoutCommand{UserID: sessionUserID, RefreshToken: pair.RefreshToken}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if sessions.revokedUser != sessionUserID || sessions.revokedHash != hashRefreshToken(pair.RefreshToken) {
		t.Fatalf("unexpected revoke: user=%q hash=%q", sessions.revokedUser, sessions.revokedHash)
	}
}

func TestLogoutRejectsForeignOrWrongToken(t *testing.T) {
	service, sessions, tokens := newSessionTestService(t, domain.User{ID: sessionUserID})
	own := issueRefreshToken(t, tokens, sessionUserID, security.UserPrincipal)
	foreign := issueRefreshToken(t, tokens, "00000000-0000-0000-0000-000000000000", security.UserPrincipal)

	for name, command := range map[string]LogoutCommand{
		"foreign refresh": {UserID: sessionUserID, RefreshToken: foreign.RefreshToken},
		"access token":    {UserID: sessionUserID, RefreshToken: own.AccessToken},
		"no user":         {RefreshToken: own.RefreshToken},
	} {
		t.Run(name, func(t *testing.T) {
			if err := service.Logout(context.Background(), command); !errors.Is(err, ErrInvalidRefreshToken) {
				t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
			}
		})
	}
	if sessions.revokeCalls != 0 {
		t.Fatal("rejected logout must not revoke any session")
	}
}
