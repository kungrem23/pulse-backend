package auth

import (
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Age      int    `json:"age"`
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

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Age       int       `json:"age"`
	IsBlocked bool      `json:"is_blocked"`
	CreatedAt time.Time `json:"created_at"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Email:     user.Email,
		Age:       user.Age,
		IsBlocked: user.IsBlocked,
		CreatedAt: user.CreatedAt.UTC(),
	}
}

type registerResponse struct {
	User    userResponse    `json:"user"`
	Session sessionResponse `json:"session"`
}
