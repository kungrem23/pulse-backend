package domain

import "context"

// Интерфейсы принадлежат бизнес-слою; PostgreSQL и S3 реализуют их снаружи.
type AdminRepository interface {
	ByUsername(context.Context, string) (Admin, error)
}

type UserRepository interface {
	// Create stores Age as users.age; it does not require a completed profile.
	Create(context.Context, User) (User, error)
	ByEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
}

type RefreshSessionRepository interface {
	Create(context.Context, RefreshSession) error
	// Rotate atomically revokes the user's active, unexpired session with
	// oldTokenHash and stores next. Returns ErrNotFound when no such session
	// exists; of concurrent rotations of one session, only one succeeds.
	Rotate(ctx context.Context, oldTokenHash string, next RefreshSession) error
	// Revoke is idempotent: an absent or already revoked session is not an error.
	Revoke(ctx context.Context, userID, tokenHash string) error
}

type ProfileRepository interface {
	// Upsert updates users.age and the profile/preferences in one transaction.
	Upsert(context.Context, Profile, Preferences) error
	ByUserID(context.Context, string) (Profile, error)
	Feed(context.Context, string, PageRequest) (Page[Profile], error)
}

type SwipeRepository interface {
	// Save atomically stores a swipe and a unique match. Closed pairs cannot rematch.
	Save(context.Context, string, string, bool) (*Match, error)
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
