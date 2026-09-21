package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const minimumJWTSecretLength = 32

var (
	ErrInvalidToken        = errors.New("invalid token")
	ErrUnexpectedTokenType = errors.New("unexpected token type")
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type PrincipalType string

const (
	UserPrincipal  PrincipalType = "user"
	AdminPrincipal PrincipalType = "admin"
)

type TokenIssuerConfig struct {
	Secret          string
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Claims struct {
	TokenType TokenType     `json:"token_type"`
	Principal PrincipalType `json:"principal"`
	jwt.RegisteredClaims
}

// TokenIssuer creates and verifies signed access and refresh JWTs.
type TokenIssuer struct {
	secret          []byte
	issuer          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	now             func() time.Time
}

func NewTokenIssuer(config TokenIssuerConfig) (*TokenIssuer, error) {
	if len(config.Secret) < minimumJWTSecretLength {
		return nil, fmt.Errorf("JWT secret must contain at least %d bytes", minimumJWTSecretLength)
	}
	if strings.TrimSpace(config.Issuer) == "" {
		return nil, errors.New("JWT issuer must not be empty")
	}
	if config.AccessTokenTTL <= 0 {
		return nil, errors.New("access token TTL must be positive")
	}
	if config.RefreshTokenTTL <= 0 {
		return nil, errors.New("refresh token TTL must be positive")
	}
	if config.RefreshTokenTTL <= config.AccessTokenTTL {
		return nil, errors.New("refresh token TTL must be greater than access token TTL")
	}

	return &TokenIssuer{
		secret:          []byte(config.Secret),
		issuer:          config.Issuer,
		accessTokenTTL:  config.AccessTokenTTL,
		refreshTokenTTL: config.RefreshTokenTTL,
		now:             time.Now,
	}, nil
}

// Issue creates an access/refresh pair for a user or administrator.
func (i *TokenIssuer) Issue(subject string, principal PrincipalType) (TokenPair, error) {
	if i == nil {
		return TokenPair{}, errors.New("token issuer is nil")
	}
	if strings.TrimSpace(subject) == "" {
		return TokenPair{}, errors.New("token subject must not be empty")
	}
	if !principal.valid() {
		return TokenPair{}, fmt.Errorf("invalid principal type %q", principal)
	}

	now := i.now().UTC()
	accessExpiresAt := now.Add(i.accessTokenTTL)
	refreshExpiresAt := now.Add(i.refreshTokenTTL)

	accessToken, err := i.issueToken(subject, principal, AccessToken, now, accessExpiresAt)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}
	refreshToken, err := i.issueToken(subject, principal, RefreshToken, now, refreshExpiresAt)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// Verify validates the signature, algorithm, issuer, expiration, token type,
// and application-specific claims. Callers must state which token type they
// expect so a refresh token cannot be used as an access token.
func (i *TokenIssuer) Verify(rawToken string, expectedType TokenType) (Claims, error) {
	if i == nil {
		return Claims{}, errors.New("token issuer is nil")
	}
	if expectedType != AccessToken && expectedType != RefreshToken {
		return Claims{}, fmt.Errorf("invalid expected token type %q", expectedType)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method %q", token.Method.Alg())
			}
			return i.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil || token == nil || !token.Valid {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.TokenType != expectedType {
		return Claims{}, fmt.Errorf("%w: expected %q, got %q", ErrUnexpectedTokenType, expectedType, claims.TokenType)
	}
	if strings.TrimSpace(claims.Subject) == "" ||
		claims.ID == "" ||
		claims.ExpiresAt == nil ||
		claims.IssuedAt == nil ||
		claims.NotBefore == nil ||
		!claims.Principal.valid() {
		return Claims{}, fmt.Errorf("%w: required claims are missing", ErrInvalidToken)
	}

	return *claims, nil
}

func (i *TokenIssuer) issueToken(
	subject string,
	principal PrincipalType,
	tokenType TokenType,
	issuedAt time.Time,
	expiresAt time.Time,
) (string, error) {
	tokenID, err := newTokenID()
	if err != nil {
		return "", err
	}

	claims := Claims{
		TokenType: tokenType,
		Principal: principal,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ID:        tokenID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signedToken, nil
}

func newTokenID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate token ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func (p PrincipalType) valid() bool {
	return p == UserPrincipal || p == AdminPrincipal
}
