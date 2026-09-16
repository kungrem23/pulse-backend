// Package auth describes registration and authentication use cases.
package auth

import "context"

type RegisterCommand struct {
	Email    string
	Password string
}

type LoginCommand struct {
	Login    string
	Password string
}

type Session struct {
	AccessToken  string
	RefreshToken string
}

type Service interface {
	Register(context.Context, RegisterCommand) (Session, error)
	Login(context.Context, LoginCommand) (Session, error)
	LoginAdmin(context.Context, LoginCommand) (Session, error)
}
