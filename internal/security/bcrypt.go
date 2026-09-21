package security

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var ErrPasswordMismatch = errors.New("password does not match hash")

// PasswordManager generates and verifies bcrypt password hashes.
// Password requirements (length, complexity, and so on) belong to the
// application layer; this type is only responsible for secure storage.
type PasswordManager struct {
	cost int
}

// NewPasswordManager creates a password manager with an explicit bcrypt cost.
// Use bcrypt.DefaultCost in production and bcrypt.MinCost in fast unit tests.
func NewPasswordManager(cost int) (*PasswordManager, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	return &PasswordManager{cost: cost}, nil
}

// NewDefaultPasswordManager creates a password manager using bcrypt's default
// work factor.
func NewDefaultPasswordManager() *PasswordManager {
	manager, err := NewPasswordManager(bcrypt.DefaultCost)
	if err != nil {
		// The library's default cost is guaranteed to be valid.
		panic(err)
	}
	return manager
}

// Hash returns a bcrypt hash suitable for storing in the database.
func (m *PasswordManager) Hash(password string) (string, error) {
	if m == nil {
		return "", errors.New("password manager is nil")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), m.cost)
	if err != nil {
		return "", fmt.Errorf("generate password hash: %w", err)
	}
	return string(hash), nil
}

// Verify checks a plaintext password against an encoded bcrypt hash.
// ErrPasswordMismatch is returned when the password is incorrect.
func (m *PasswordManager) Verify(password, encodedHash string) error {
	if m == nil {
		return errors.New("password manager is nil")
	}

	err := bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}
	return fmt.Errorf("verify password hash: %w", err)
}
