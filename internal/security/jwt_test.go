package security

import (
	"errors"
	"testing"
	"time"
)

func TestTokenIssuerIssueAndVerify(t *testing.T) {
	issuer := newTestTokenIssuer(t)
	fixedNow := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	issuer.now = func() time.Time { return fixedNow }

	pair, err := issuer.Issue("user-123", UserPrincipal)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Fatal("access and refresh tokens must differ")
	}
	if want := fixedNow.Add(15 * time.Minute); !pair.AccessExpiresAt.Equal(want) {
		t.Fatalf("access expiration: want %v, got %v", want, pair.AccessExpiresAt)
	}

	claims, err := issuer.Verify(pair.AccessToken, AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Fatalf("subject: want user-123, got %q", claims.Subject)
	}
	if claims.Principal != UserPrincipal {
		t.Fatalf("principal: want %q, got %q", UserPrincipal, claims.Principal)
	}
	if claims.TokenType != AccessToken {
		t.Fatalf("token type: want %q, got %q", AccessToken, claims.TokenType)
	}
	if claims.ID == "" {
		t.Fatal("token ID must not be empty")
	}
}

func TestTokenIssuerRejectsWrongTokenType(t *testing.T) {
	issuer := newTestTokenIssuer(t)
	pair, err := issuer.Issue("user-123", UserPrincipal)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	_, err = issuer.Verify(pair.RefreshToken, AccessToken)
	if !errors.Is(err, ErrUnexpectedTokenType) {
		t.Fatalf("expected ErrUnexpectedTokenType, got %v", err)
	}
}

func TestTokenIssuerRejectsExpiredToken(t *testing.T) {
	issuer := newTestTokenIssuer(t)
	fixedNow := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	issuer.now = func() time.Time { return fixedNow }
	pair, err := issuer.Issue("admin-123", AdminPrincipal)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	issuer.now = func() time.Time { return fixedNow.Add(16 * time.Minute) }
	_, err = issuer.Verify(pair.AccessToken, AccessToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestTokenIssuerRejectsTokenSignedWithAnotherSecret(t *testing.T) {
	issuer := newTestTokenIssuer(t)
	otherIssuer, err := NewTokenIssuer(TokenIssuerConfig{
		Secret:          "a-different-secret-with-at-least-32-bytes",
		Issuer:          "pulse-backend",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create other issuer: %v", err)
	}
	pair, err := otherIssuer.Issue("user-123", UserPrincipal)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	_, err = issuer.Verify(pair.AccessToken, AccessToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestTokenIssuerValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config TokenIssuerConfig
	}{
		{name: "short secret", config: TokenIssuerConfig{Secret: "short", Issuer: "pulse", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}},
		{name: "empty issuer", config: TokenIssuerConfig{Secret: "a-secret-with-at-least-thirty-two-bytes", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}},
		{name: "invalid access TTL", config: TokenIssuerConfig{Secret: "a-secret-with-at-least-thirty-two-bytes", Issuer: "pulse", RefreshTokenTTL: time.Hour}},
		{name: "short refresh TTL", config: TokenIssuerConfig{Secret: "a-secret-with-at-least-thirty-two-bytes", Issuer: "pulse", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Minute}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewTokenIssuer(test.config); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func newTestTokenIssuer(t *testing.T) *TokenIssuer {
	t.Helper()
	issuer, err := NewTokenIssuer(TokenIssuerConfig{
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
