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

// PhotoRepository stores private object keys. Implementations must enforce
// ownership in read/delete operations and map a duplicate (user, position)
// to ErrConflict. SQL details belong to the PostgreSQL adapter.
type PhotoRepository interface {
	Create(context.Context, Photo) error
	ByIDForUser(context.Context, string, string) (Photo, error)
	// ByUserID lists only this user's photos ordered by position ASC.
	// No photos is an empty slice, not ErrNotFound. Keys remain internal;
	// application services obtain fresh URLs through PhotoStorage.
	ByUserID(ctx context.Context, userID string) ([]Photo, error)
	Delete(context.Context, string, string) error
}

// SearchStateRepository supplies the common eligibility precheck for feed/swipe.
// An existing user without a profile/preferences/photos gets a partial state;
// only a missing user returns ErrNotFound. A precheck is not a write-time lock.
type SearchStateRepository interface {
	StateByUserID(ctx context.Context, userID string) (SearchState, error)
}

type DiscoveryRepository interface {
	// Feed applies mutual age/sex preferences, with no city filter, and excludes
	// self, ineligible users, outgoing swipes, and all active/closed match pairs.
	// Order: users.created_at DESC, users.id DESC; After is an exclusive boundary.
	// Return at most Limit candidates and HasMore using one-row lookahead.
	// Photos are included in position ASC order, without per-candidate queries.
	// Recheck the caller's eligibility while reading; return CheckEligible's
	// error, or ErrNotFound if the caller no longer exists.
	Feed(ctx context.Context, userID string, page PageQuery) (PageResult[DiscoveryCandidate], error)
}

type SwipeRepository interface {
	// Save serializes the canonical UUID pair and atomically stores the swipe
	// and, for mutual likes, exactly one match. It rechecks both users' state,
	// mutual preferences, outgoing swipe history and any existing match inside
	// the transaction; an earlier app-layer precheck cannot replace this.
	// Existing outgoing swipe or active/closed pair returns ErrConflict without
	// rewriting history. Missing/ineligible/nonmatching target returns
	// ErrCandidateUnavailable; an ineligible caller gets CheckEligible's error,
	// a missing caller ErrNotFound, and a self-swipe ErrSelfSwipe.
	// Return nil for no match, or the created match; never create a conversation.
	Save(ctx context.Context, fromUserID, toUserID string, isLike bool) (*Match, error)
}

type MatchRepository interface {
	// ListByUserID returns only this user's matches, including closed ones,
	// ordered by matches.created_at DESC, matches.id DESC. After is exclusive.
	// Each result includes the other user's profile and optional first photo
	// (lowest position); do not apply discovery eligibility to existing matches.
	// Return at most Limit entries and HasMore using one-row lookahead.
	// Partner data is loaded in bulk, never by querying once per match.
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
