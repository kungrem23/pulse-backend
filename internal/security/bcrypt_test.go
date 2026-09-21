package security

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordManagerHashAndVerify(t *testing.T) {
	manager, err := NewPasswordManager(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("create password manager: %v", err)
	}

	hash, err := manager.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("expected a bcrypt hash, got %q", hash)
	}
	if err := manager.Verify("correct horse battery staple", hash); err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if err := manager.Verify("wrong password", hash); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}

func TestPasswordManagerRejectsInvalidCost(t *testing.T) {
	if _, err := NewPasswordManager(bcrypt.MinCost - 1); err == nil {
		t.Fatal("expected invalid cost error")
	}
}

func TestPasswordManagerRejectsMalformedHash(t *testing.T) {
	manager, err := NewPasswordManager(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("create password manager: %v", err)
	}

	err = manager.Verify("password", "not-a-bcrypt-hash")
	if err == nil || errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected malformed hash error, got %v", err)
	}
}
