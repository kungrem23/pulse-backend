package auth

import (
	"context"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/security"
	"golang.org/x/crypto/bcrypt"
)

type userRepositoryStub struct {
	createdUser    domain.User
	createErr      error
	byEmailUser    domain.User
	byEmailErr     error
	createCalls    int
	byEmailCalls   int
	createInput    domain.User
	requestedEmail string
	byIDUser       domain.User
}

func (r *userRepositoryStub) Create(_ context.Context, user domain.User) (domain.User, error) {
	r.createCalls++
	r.createInput = user
	return r.createdUser, r.createErr
}

func (r *userRepositoryStub) ByEmail(_ context.Context, email string) (domain.User, error) {
	r.byEmailCalls++
	r.requestedEmail = email
	return r.byEmailUser, r.byEmailErr
}

func (r *userRepositoryStub) ByID(_ context.Context, id string) (domain.User, error) {
	if r.byIDUser.ID == "" || r.byIDUser.ID != id {
		return domain.User{}, domain.ErrNotFound
	}
	return r.byIDUser, nil
}

type sessionRepositoryStub struct {
	created     []domain.RefreshSession
	createErr   error
	rotateErr   error
	rotateCalls int
	rotatedHash string
	rotatedNext domain.RefreshSession
	revokeCalls int
	revokedUser string
	revokedHash string
}

func (r *sessionRepositoryStub) Create(_ context.Context, session domain.RefreshSession) error {
	r.created = append(r.created, session)
	return r.createErr
}

func (r *sessionRepositoryStub) Rotate(_ context.Context, oldTokenHash string, next domain.RefreshSession) error {
	r.rotateCalls++
	r.rotatedHash = oldTokenHash
	r.rotatedNext = next
	return r.rotateErr
}

func (r *sessionRepositoryStub) Revoke(_ context.Context, userID, tokenHash string) error {
	r.revokeCalls++
	r.revokedUser = userID
	r.revokedHash = tokenHash
	return nil
}

type passwordManagerStub struct {
	hash        string
	hashErr     error
	verifyErr   error
	hashCalls   int
	verifyCalls int
	password    string
	encodedHash string
}

func (m *passwordManagerStub) Hash(password string) (string, error) {
	m.hashCalls++
	m.password = password
	return m.hash, m.hashErr
}

func (m *passwordManagerStub) Verify(password, encodedHash string) error {
	m.verifyCalls++
	m.password = password
	m.encodedHash = encodedHash
	return m.verifyErr
}

type tokenIssuerStub struct {
	pair      security.TokenPair
	err       error
	calls     int
	subject   string
	principal security.PrincipalType
}

func (i *tokenIssuerStub) Issue(subject string, principal security.PrincipalType) (security.TokenPair, error) {
	i.calls++
	i.subject = subject
	i.principal = principal
	return i.pair, i.err
}

func (i *tokenIssuerStub) Verify(string, security.TokenType) (security.Claims, error) {
	return security.Claims{}, security.ErrInvalidToken
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
