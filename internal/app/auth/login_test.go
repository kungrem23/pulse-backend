package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginIssuesUserSession(t *testing.T) {
	passwords := newTestPasswordManager(t)
	passwordHash, err := passwords.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	tokens := newTestTokenIssuer(t)
	repository := &userRepositoryStub{
		byEmailUser: domain.User{
			ID:           "6130ce38-a239-447c-b65a-a2019b370962",
			Email:        "user@example.com",
			PasswordHash: passwordHash,
		},
	}
	service := NewService(repository, passwords, tokens)

	session, err := service.Login(context.Background(), LoginCommand{
		Email:    "  User@Example.COM  ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if repository.requestedEmail != "user@example.com" {
		t.Fatalf("repository email: want user@example.com, got %q", repository.requestedEmail)
	}
	claims, err := tokens.Verify(session.AccessToken, security.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if claims.Subject != repository.byEmailUser.ID || claims.Principal != security.UserPrincipal {
		t.Fatalf("unexpected access claims: subject=%q principal=%q", claims.Subject, claims.Principal)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	passwords := newTestPasswordManager(t)
	passwordHash, err := passwords.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	service := NewService(
		&userRepositoryStub{byEmailUser: domain.User{ID: "user-id", PasswordHash: passwordHash}},
		passwords,
		newTestTokenIssuer(t),
	)

	_, err = service.Login(context.Background(), LoginCommand{
		Email:    "user@example.com",
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginHidesUnknownEmail(t *testing.T) {
	passwords := &passwordManagerStub{verifyErr: security.ErrPasswordMismatch}
	service := NewService(
		&userRepositoryStub{byEmailErr: domain.ErrNotFound},
		passwords,
		&tokenIssuerStub{},
	)

	_, err := service.Login(context.Background(), LoginCommand{
		Email:    "unknown@example.com",
		Password: "password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if passwords.verifyCalls != 1 || passwords.encodedHash != dummyUserPasswordHash {
		t.Fatal("expected one dummy password verification")
	}
}

func TestDummyUserPasswordHashUsesProductionCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyUserPasswordHash))
	if err != nil {
		t.Fatalf("dummy password hash must be valid bcrypt: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("dummy hash cost: want %d, got %d", bcrypt.DefaultCost, cost)
	}
}

func TestLoginReturnsBlockedOnlyForCorrectPassword(t *testing.T) {
	passwords := newTestPasswordManager(t)
	passwordHash, err := passwords.Hash("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	service := NewService(
		&userRepositoryStub{byEmailUser: domain.User{ID: "user-id", PasswordHash: passwordHash, IsBlocked: true}},
		passwords,
		&tokenIssuerStub{},
	)

	_, err = service.Login(context.Background(), LoginCommand{Email: "user@example.com", Password: "wrong-password"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: expected ErrInvalidCredentials, got %v", err)
	}

	_, err = service.Login(context.Background(), LoginCommand{Email: "user@example.com", Password: "correct-password"})
	if !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("correct password: expected ErrUserBlocked, got %v", err)
	}
}
