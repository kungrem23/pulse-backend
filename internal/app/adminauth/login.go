package adminauth

import "context"

func (s *service) Login(ctx context.Context, command LoginCommand) (Session, error) {
	admin, err := s.admins.ByUsername(ctx, command.Username)
	if err != nil {
		return Session{}, err
	}
	if 
	return Session{}, nil
}
