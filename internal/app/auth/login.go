package auth

import "context"

func (s *service) Login(ctx context.Context, command LoginCommand) (Session, error) {
	return Session{}, nil
}
