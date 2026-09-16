package domain

import "time"

type Sex string

const (
	SexMale   Sex = "male"
	SexFemale Sex = "female"
	SexOther  Sex = "other"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	IsBlocked    bool      `json:"is_blocked"`
	CreatedAt    time.Time `json:"created_at"`
}

type Profile struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Bio        string `json:"bio"`
	BirthDate  string `json:"birth_date"`
	Sex        Sex    `json:"sex"`
	City       string `json:"city"`
	IsSleeping bool   `json:"is_sleeping"`
}

type Preferences struct {
	UserID string `json:"user_id"`
	MinAge int16  `json:"min_age"`
	MaxAge int16  `json:"max_age"`
	Sex    Sex    `json:"sex"`
}

type Photo struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ObjectKey string `json:"object_key"`
	Position  int16  `json:"position"`
}

type Match struct {
	ID        string    `json:"id"`
	Person1ID string    `json:"person1_id"`
	Person2ID string    `json:"person2_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID        string    `json:"id"`
	MatchID   *string   `json:"match_id,omitempty"`
	FromID    string    `json:"from_id"`
	ToID      string    `json:"to_id"`
	Text      string    `json:"text"`
	ReplyToID *string   `json:"reply_to_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Report struct {
	ID          string    `json:"id"`
	FromID      string    `json:"from_id"`
	ToID        string    `json:"to_id"`
	Reason      string    `json:"reason"`
	IsProcessed bool      `json:"is_processed"`
	CreatedAt   time.Time `json:"created_at"`
}
