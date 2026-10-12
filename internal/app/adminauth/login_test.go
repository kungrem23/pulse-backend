package adminauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginIssuesAdminSession(t *testing.T) {
	passwords := newTestPasswordManager(t)
	passwordHash, err := passwords.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	tokens := newTestTokenIssuer(t)
	repository := &adminRepositoryStub{
		admin: domain.Admin{
			ID:           "5ec55d5f-7269-4278-bd26-4f85b560e0bc",
			Username:     "moderator",
			PasswordHash: passwordHash,
		},
	}
	service := NewService(repository, passwords, tokens)

	session, err := service.Login(context.Background(), LoginCommand{
		Username: "  moderator  ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if repository.requestedUsername != "moderator" {
		t.Fatalf("repository username: want moderator, got %q", repository.requestedUsername)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}
	if session.AccessExpiresAt.IsZero() || session.RefreshExpiresAt.IsZero() {
		t.Fatal("expected token expiration times")
	}

	accessClaims, err := tokens.Verify(session.AccessToken, security.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if accessClaims.Subject != repository.admin.ID {
		t.Fatalf("token subject: want %q, got %q", repository.admin.ID, accessClaims.Subject)
	}
	if accessClaims.Principal != security.AdminPrincipal {
		t.Fatalf("token principal: want %q, got %q", security.AdminPrincipal, accessClaims.Principal)
	}

	refreshClaims, err := tokens.Verify(session.RefreshToken, security.RefreshToken)
	if err != nil {
		t.Fatalf("verify refresh token: %v", err)
	}
	if refreshClaims.Principal != security.AdminPrincipal {
		t.Fatalf("refresh principal: want %q, got %q", security.AdminPrincipal, refreshClaims.Principal)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	passwords := newTestPasswordManager(t)
	passwordHash, err := passwords.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	service := NewService(&adminRepositoryStub{
		admin: domain.Admin{ID: "admin-id", Username: "moderator", PasswordHash: passwordHash},
	}, passwords, newTestTokenIssuer(t))

	_, err = service.Login(context.Background(), LoginCommand{
		Username: "moderator",
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginHidesUnknownUsername(t *testing.T) {
	passwords := &passwordVerifierStub{err: security.ErrPasswordMismatch}
	service := NewService(
		&adminRepositoryStub{err: domain.ErrNotFound},
		passwords,
		&tokenIssuerStub{},
	)

	_, err := service.Login(context.Background(), LoginCommand{
		Username: "unknown",
		Password: "password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if passwords.calls != 1 || passwords.encodedHash != dummyAdminPasswordHash {
		t.Fatal("expected one dummy password verification")
	}
}

func TestDummyAdminPasswordHashUsesProductionCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyAdminPasswordHash))
	if err != nil {
		t.Fatalf("dummy password hash must be valid bcrypt: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("dummy hash cost: want %d, got %d", bcrypt.DefaultCost, cost)
	}
}

func TestLoginDoesNotQueryRepositoryForEmptyCredentials(t *testing.T) {
	repository := &adminRepositoryStub{}
	service := NewService(repository, &passwordVerifierStub{}, &tokenIssuerStub{})

	_, err := service.Login(context.Background(), LoginCommand{})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls: want 0, got %d", repository.calls)
	}
}

func TestLoginPropagatesRepositoryFailure(t *testing.T) {
	repositoryError := errors.New("database unavailable")
	service := NewService(
		&adminRepositoryStub{err: repositoryError},
		&passwordVerifierStub{},
		&tokenIssuerStub{},
	)

	_, err := service.Login(context.Background(), LoginCommand{
		Username: "moderator",
		Password: "password",
	})
	if !errors.Is(err, repositoryError) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestLoginPropagatesTokenIssuerFailure(t *testing.T) {
	issuerError := errors.New("signing failed")
	service := NewService(
		&adminRepositoryStub{admin: domain.Admin{ID: "admin-id", PasswordHash: "hash"}},
		&passwordVerifierStub{},
		&tokenIssuerStub{err: issuerError},
	)

	_, err := service.Login(context.Background(), LoginCommand{
		Username: "moderator",
		Password: "password",
	})
	if !errors.Is(err, issuerError) {
		t.Fatalf("expected token issuer error, got %v", err)
	}
}

type adminRepositoryStub struct {
	admin             domain.Admin
	err               error
	calls             int
	requestedUsername string
}

func (r *adminRepositoryStub) ByUsername(_ context.Context, username string) (domain.Admin, error) {
	r.calls++
	r.requestedUsername = username
	return r.admin, r.err
}

type passwordVerifierStub struct {
	err         error
	calls       int
	password    string
	encodedHash string
}

func (v *passwordVerifierStub) Verify(password, encodedHash string) error {
	v.calls++
	v.password = password
	v.encodedHash = encodedHash
	return v.err
}

type tokenIssuerStub struct {
	pair      security.TokenPair
	err       error
	subject   string
	principal security.PrincipalType
}

func (i *tokenIssuerStub) Issue(subject string, principal security.PrincipalType) (security.TokenPair, error) {
	i.subject = subject
	i.principal = principal
	return i.pair, i.err
}

func newTestPasswordManager(t *testing.T) *security.PasswordManager {
	t.Helper()
	manager, err := security.NewPasswordManager(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("create password manager: %v", err)
	}
	return manager
}

func newTestTokenIssuer(t *testing.T) *security.TokenIssuer {
	t.Helper()
	issuer, err := security.NewTokenIssuer(security.TokenIssuerConfig{
		Secret:          "test-secret-with-at-least-thirty-two-bytes",
		Issuer:          "pulse-backend",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create token issuer: %v", err)
	}
	return issuer
}
