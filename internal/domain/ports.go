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
	// Save creates or replaces the profile and sets users.age in one transaction.
	// IsSleeping is ignored on input: an existing profile keeps its state, a new
	// one starts awake. Returns ErrNotFound if the user does not exist.
	Save(context.Context, Profile) (Profile, error)
	// ByUserID returns the profile with Age read from users.age.
	ByUserID(context.Context, string) (Profile, error)
	// Preferences returns ErrNotFound until the user has set them.
	Preferences(context.Context, string) (Preferences, error)
	// SavePreferences creates or replaces preferences. Returns ErrNotFound if
	// the user has no profile yet.
	SavePreferences(context.Context, Preferences) (Preferences, error)
	// SetSleep returns the stored state, or ErrNotFound if there is no profile.
	SetSleep(ctx context.Context, userID string, isSleeping bool) (bool, error)
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
