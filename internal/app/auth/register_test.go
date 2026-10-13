package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
)

func TestRegisterCreatesUserAndIssuesSession(t *testing.T) {
	passwords := newTestPasswordManager(t)
	tokens := newTestTokenIssuer(t)
	repository := &userRepositoryStub{
		createdUser: domain.User{
			ID:           "6130ce38-a239-447c-b65a-a2019b370962",
			Email:        "user@example.com",
			PasswordHash: "must-not-leave-the-service",
			CreatedAt:    time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC),
		},
	}
	service := NewService(repository, &sessionRepositoryStub{}, passwords, tokens)

	registration, err := service.Register(context.Background(), RegisterCommand{
		Email:    "  User@Example.COM  ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if repository.createInput.Email != "user@example.com" {
		t.Fatalf("created email: want user@example.com, got %q", repository.createInput.Email)
	}
	if repository.createInput.Age != nil || registration.User.Age != nil {
		t.Fatal("registration must leave age unset until profile creation")
	}
	if repository.createInput.PasswordHash == "" || repository.createInput.PasswordHash == "correct-password" {
		t.Fatal("repository must receive a password hash, not plaintext")
	}
	if err := passwords.Verify("correct-password", repository.createInput.PasswordHash); err != nil {
		t.Fatalf("verify stored password hash: %v", err)
	}
	if registration.User.PasswordHash != "" {
		t.Fatal("registration result must not expose password hash")
	}
	if registration.Session.AccessToken == "" || registration.Session.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}
	claims, err := tokens.Verify(registration.Session.AccessToken, security.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if claims.Subject != registration.User.ID || claims.Principal != security.UserPrincipal {
		t.Fatalf("unexpected access claims: subject=%q principal=%q", claims.Subject, claims.Principal)
	}
}

func TestRegisterValidatesInputBeforeHashing(t *testing.T) {
	tests := []struct {
		name     string
		command  RegisterCommand
		expected error
	}{
		{name: "invalid email", command: RegisterCommand{Email: "not-an-email", Password: "correct-password"}, expected: ErrInvalidEmail},
		{name: "short password", command: RegisterCommand{Email: "user@example.com", Password: "short"}, expected: ErrInvalidPassword},
		{name: "bcrypt byte limit", command: RegisterCommand{Email: "user@example.com", Password: strings.Repeat("я", 37)}, expected: ErrInvalidPassword},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &userRepositoryStub{}
			passwords := &passwordManagerStub{}
			service := NewService(repository, &sessionRepositoryStub{}, passwords, &tokenIssuerStub{})

			_, err := service.Register(context.Background(), test.command)
			if !errors.Is(err, test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, err)
			}
			if passwords.hashCalls != 0 || repository.createCalls != 0 {
				t.Fatal("invalid input must not reach hashing or repository")
			}
		})
	}
}

func TestRegisterMapsEmailConflict(t *testing.T) {
	service := NewService(
		&userRepositoryStub{createErr: domain.ErrConflict},
		&sessionRepositoryStub{},
		&passwordManagerStub{hash: "password-hash"},
		&tokenIssuerStub{},
	)

	_, err := service.Register(context.Background(), RegisterCommand{
		Email:    "user@example.com",
		Password: "correct-password",
	})
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestRegisterPropagatesTokenIssuerFailure(t *testing.T) {
	issuerError := errors.New("signing failed")
	service := NewService(
		&userRepositoryStub{createdUser: domain.User{ID: "user-id"}},
		&sessionRepositoryStub{},
		&passwordManagerStub{hash: "password-hash"},
		&tokenIssuerStub{err: issuerError},
	)

	_, err := service.Register(context.Background(), RegisterCommand{
		Email:    "user@example.com",
		Password: "correct-password",
	})
	if !errors.Is(err, issuerError) {
		t.Fatalf("expected token issuer error, got %v", err)
	}
}
