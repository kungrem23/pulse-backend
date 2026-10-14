package domain

import "context"

type AdminRepository interface {
	ByUsername(context.Context, string) (Admin, error)
}

type UserRepository interface {
	Create(context.Context, User) (User, error)
	ByEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
}

type RefreshSessionRepository interface {
	Create(context.Context, RefreshSession) error
	Rotate(ctx context.Context, oldTokenHash string, next RefreshSession) error
	Revoke(ctx context.Context, userID, tokenHash string) error
}

type ProfileRepository interface {
	Save(context.Context, Profile) (Profile, error)
	ByUserID(context.Context, string) (Profile, error)
	Preferences(context.Context, string) (Preferences, error)
	SavePreferences(context.Context, Preferences) (Preferences, error)
	SetSleep(ctx context.Context, userID string, isSleeping bool) (bool, error)
}

type PhotoRepository interface {
	Create(context.Context, Photo) error
	ByIDForUser(context.Context, string, string) (Photo, error)
	ByUserID(ctx context.Context, userID string) ([]Photo, error)
	Delete(context.Context, string, string) error
}

type SearchStateRepository interface {
	StateByUserID(ctx context.Context, userID string) (SearchState, error)
}

type DiscoveryRepository interface {
	Feed(ctx context.Context, userID string, page PageQuery) (PageResult[DiscoveryCandidate], error)
}

type SwipeRepository interface {
	Save(ctx context.Context, fromUserID, toUserID string, isLike bool) (*Match, error)
}

type MatchRepository interface {
	ListByUserID(ctx context.Context, userID string, page PageQuery) (PageResult[MatchWithPartner], error)
}

type MessageRepository interface {
	List(context.Context, string, string, PageRequest) (Page[Message], error)
	Create(context.Context, Message) error
}

type ReportRepository interface {
	Create(context.Context, Report) error
	List(context.Context, string, PageRequest) (Page[Report], error)
	Process(context.Context, Report) error
}

type PhotoStorage interface {
	Put(context.Context, string, []byte, string) (string, error)
	Delete(context.Context, string) error
	URL(context.Context, string) (string, error)
}
