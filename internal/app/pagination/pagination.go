// Package pagination defines the shared MVP cursor contract for feed/matches.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	DefaultLimit    = 20
	MaxLimit        = 100
	MaxCursorLength = 512
	cursorVersion   = 1
)

type Scope string

const (
	Feed    Scope = "feed"
	Matches Scope = "matches"
)

// wireCursor is base64url without padding over this JSON object. Its position
// uses RFC3339Nano in UTC and a canonical UUID. Scope and owner prevent accidental
// reuse between lists/users; the cursor is not signed and grants no access.
type wireCursor struct {
	Version   int       `json:"v"`
	Scope     Scope     `json:"scope"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// Parse validates app input and decodes a cursor into a repository query.
// Limit == 0 means omitted; the HTTP adapter must reject an explicit ?limit=0.
// An empty cursor means omitted; HTTP must reject an explicit ?cursor=.
func Parse(request domain.PageRequest, scope Scope, userID string) (domain.PageQuery, error) {
	owner, ok := canonicalUUID(userID)
	if !ok {
		return domain.PageQuery{}, domain.ErrInvalidUserID
	}
	if !validScope(scope) {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	limit := request.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return domain.PageQuery{}, domain.ErrInvalidLimit
	}
	page := domain.PageQuery{Limit: limit}
	if request.Cursor == "" {
		return page, nil
	}
	if len(request.Cursor) > MaxCursorLength {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	content, err := base64.RawURLEncoding.Strict().DecodeString(request.Cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(content) != request.Cursor {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	var cursor wireCursor
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	id, validID := canonicalUUID(cursor.ID)
	if cursor.Version != cursorVersion || cursor.Scope != scope || cursor.UserID != owner ||
		!validID || cursor.CreatedAt.IsZero() {
		return domain.PageQuery{}, domain.ErrInvalidCursor
	}
	page.After = &domain.PagePosition{CreatedAt: cursor.CreatedAt.UTC(), ID: id}
	return page, nil
}

// Encode uses the last returned item's position only if the repository HasMore.
// Feed position = (users.created_at, users.id); match position =
// (matches.created_at, matches.id). Both lists sort descending on both fields.
func Encode(position domain.PagePosition, scope Scope, userID string) (string, error) {
	owner, ok := canonicalUUID(userID)
	if !ok {
		return "", domain.ErrInvalidUserID
	}
	id, ok := canonicalUUID(position.ID)
	if !ok || position.CreatedAt.IsZero() || !validScope(scope) {
		return "", domain.ErrInvalidCursor
	}
	content, err := json.Marshal(wireCursor{
		Version: cursorVersion, Scope: scope, UserID: owner,
		CreatedAt: position.CreatedAt.UTC(), ID: id,
	})
	if err != nil {
		return "", domain.ErrInvalidCursor
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func validScope(scope Scope) bool {
	return scope == Feed || scope == Matches
}

// UUIDs must use the standard hyphenated form, not URNs or compact hex.
func canonicalUUID(value string) (string, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != strings.ToLower(value) {
		return "", false
	}
	return parsed.String(), true
}
