package auth

import (
	"time"

	"github.com/example/pulse-backend/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type registerResponse struct {
	User    domain.User     `json:"user"`
	Session sessionResponse `json:"session"`
}
