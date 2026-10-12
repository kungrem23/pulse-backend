package domain

import "time"

type PageRequest struct {
	Cursor string
	Limit  int
}

type Page[T any] struct {
	Items      []T
	NextCursor string
}

type Sex string

const (
	SexMale   Sex = "male"
	SexFemale Sex = "female"
)

// SearchSex is separate from Sex: "any" is a preference, not a profile sex.
type SearchSex string

const (
	SearchSexMale   SearchSex = "male"
	SearchSexFemale SearchSex = "female"
	SearchSexAny    SearchSex = "any"
)

const (
	MinAge           = 18
	MaxAge           = 120
	MaxBioLength     = 500
	MaxPhotos        = 5
	MaxPhotoBytes    = 10 * 1024 * 1024
	MaxMessageLength = 2000
)

type Admin struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Age          int       `json:"age"`
	PasswordHash string    `json:"-"`
	IsBlocked    bool      `json:"is_blocked"`
	CreatedAt    time.Time `json:"created_at"`
}

type Profile struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Bio        string `json:"bio"`
	Age        int    `json:"age"`
	Sex        Sex    `json:"sex"`
	City       string `json:"city"`
	IsSleeping bool   `json:"is_sleeping"`
}

type Preferences struct {
	UserID string    `json:"user_id"`
	MinAge int16     `json:"min_age"`
	MaxAge int16     `json:"max_age"`
	Sex    SearchSex `json:"sex"`
}

type Photo struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ObjectKey string `json:"object_key"`
	Position  int16  `json:"position"`
}

type Match struct {
	ID             string    `json:"id"`
	ConversationID *string   `json:"conversation_id,omitempty"`
	Person1ID      string    `json:"person1_id"`
	Person2ID      string    `json:"person2_id"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// Conversation is a separate post-MVP resource. Closing it preserves history.
type Conversation struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Status    string     `json:"status"`
	MatchID   *string    `json:"match_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderType     string    `json:"sender_type"`
	SenderID       *string   `json:"sender_id,omitempty"`
	Text           string    `json:"text"`
	ReplyToID      *string   `json:"reply_to_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Report struct {
	ID               string     `json:"id"`
	FromUserID       string     `json:"from_user_id"`
	ToUserID         string     `json:"to_user_id"`
	Reason           string     `json:"reason"`
	Comment          string     `json:"comment,omitempty"`
	Status           string     `json:"status"`
	Resolution       *string    `json:"resolution,omitempty"`
	ModeratorComment string     `json:"moderator_comment,omitempty"`
	ProcessedBy      *string    `json:"processed_by,omitempty"`
	ProcessedAt      *time.Time `json:"processed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}
