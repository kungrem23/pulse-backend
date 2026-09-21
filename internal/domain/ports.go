package domain

import "context"

// Интерфейсы принадлежат бизнес-слою; PostgreSQL и S3 реализуют их снаружи.
type AdminRepository interface {
	ByUsername(context.Context, string) (Admin, error)
}

type UserRepository interface {
	Create(context.Context, User) (User, error)
	ByEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
}

type ProfileRepository interface {
	Upsert(context.Context, Profile, Preferences) error
	ByUserID(context.Context, string) (Profile, error)
	Feed(context.Context, string, int) ([]Profile, error)
}

type SwipeRepository interface {
	Save(context.Context, string, string, bool) (*Match, error)
}

type MessageRepository interface {
	List(context.Context, string, string, int) ([]Message, error)
	Create(context.Context, Message) error
}

type ReportRepository interface {
	Create(context.Context, Report) error
	Pending(context.Context, int) ([]Report, error)
	MarkProcessed(context.Context, string) error
}

type PhotoStorage interface {
	Put(context.Context, string, []byte, string) (string, error)
	Delete(context.Context, string) error
	URL(context.Context, string) (string, error)
}
