package pagination_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/app/pagination"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	ownerID = "11111111-1111-4111-8111-111111111111"
	otherID = "22222222-2222-4222-8222-222222222222"
	rowID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func TestLimits(t *testing.T) {
	for _, test := range []struct {
		limit int
		want  int
	}{
		{0, 20}, {1, 1}, {100, 100}, {-1, 0}, {101, 0},
	} {
		page, err := pagination.Parse(domain.PageRequest{Limit: test.limit}, pagination.Feed, ownerID)
		if test.want == 0 {
			if !errors.Is(err, domain.ErrInvalidLimit) {
				t.Fatalf("limit %d: expected ErrInvalidLimit, got %v", test.limit, err)
			}
			continue
		}
		if err != nil || page.Limit != test.want || page.After != nil {
			t.Fatalf("limit %d: got %+v, %v", test.limit, page, err)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	position := domain.PagePosition{
		CreatedAt: time.Date(2026, 10, 13, 12, 34, 56, 123456000, time.FixedZone("MSK", 3*60*60)),
		ID:        strings.ToUpper(rowID),
	}
	for _, scope := range []pagination.Scope{pagination.Feed, pagination.Matches} {
		cursor, err := pagination.Encode(position, scope, ownerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cursor) > pagination.MaxCursorLength || strings.Contains(cursor, "=") {
			t.Fatalf("cursor must fit OpenAPI and use unpadded base64url: %q", cursor)
		}
		page, err := pagination.Parse(domain.PageRequest{Cursor: cursor, Limit: 1}, scope, ownerID)
		if err != nil || page.After == nil {
			t.Fatalf("decode: %+v, %v", page, err)
		}
		if page.After.ID != rowID || !page.After.CreatedAt.Equal(position.CreatedAt) ||
			page.After.CreatedAt.Location() != time.UTC || page.Limit != 1 {
			t.Fatalf("pagination position was changed: %+v", page)
		}
	}
}

func TestCursorRejectsWrongUserAndScope(t *testing.T) {
	cursor, err := pagination.Encode(domain.PagePosition{CreatedAt: time.Now(), ID: rowID}, pagination.Feed, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		scope pagination.Scope
		owner string
	}{
		{pagination.Feed, otherID},
		{pagination.Matches, ownerID},
		{pagination.Scope("unsupported"), ownerID},
	} {
		_, err := pagination.Parse(domain.PageRequest{Cursor: cursor}, test.scope, test.owner)
		if !errors.Is(err, domain.ErrInvalidCursor) {
			t.Fatalf("scope %s / owner %s: expected ErrInvalidCursor, got %v", test.scope, test.owner, err)
		}
	}
}

func TestCursorRejectsMalformedPayloads(t *testing.T) {
	valid := map[string]any{
		"v": 1, "scope": "feed", "user_id": ownerID,
		"created_at": "2026-10-13T09:34:56.123456Z", "id": rowID,
	}
	payload := func(change func(map[string]any)) string {
		copy := make(map[string]any, len(valid))
		for key, value := range valid {
			copy[key] = value
		}
		change(copy)
		data, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	cursors := map[string]string{
		"base64":        "not base64!",
		"too long":      strings.Repeat("a", 513),
		"invalid JSON":  base64.RawURLEncoding.EncodeToString([]byte("not-json")),
		"trailing JSON": base64.RawURLEncoding.EncodeToString([]byte(`{} {}`)),
		"null":          base64.RawURLEncoding.EncodeToString([]byte(`null`)),
		"version":       payload(func(p map[string]any) { p["v"] = 2 }),
		"no version":    payload(func(p map[string]any) { delete(p, "v") }),
		"missing ID":    payload(func(p map[string]any) { delete(p, "id") }),
		"invalid UUID":  payload(func(p map[string]any) { p["id"] = "not-a-uuid" }),
		"compact UUID":  payload(func(p map[string]any) { p["id"] = strings.ReplaceAll(rowID, "-", "") }),
		"zero date":     payload(func(p map[string]any) { p["created_at"] = "0001-01-01T00:00:00Z" }),
		"invalid date":  payload(func(p map[string]any) { p["created_at"] = "yesterday" }),
		"unknown field": payload(func(p map[string]any) { p["sql"] = "unexpected" }),
	}
	for name, cursor := range cursors {
		t.Run(name, func(t *testing.T) {
			_, err := pagination.Parse(domain.PageRequest{Cursor: cursor}, pagination.Feed, ownerID)
			if !errors.Is(err, domain.ErrInvalidCursor) {
				t.Fatalf("expected ErrInvalidCursor, got %v", err)
			}
		})
	}
}

func TestCursorRejectsInvalidEncodeInput(t *testing.T) {
	valid := domain.PagePosition{CreatedAt: time.Now(), ID: rowID}
	for _, test := range []struct {
		position domain.PagePosition
		scope    pagination.Scope
		owner    string
		want     error
	}{
		{valid, pagination.Feed, "not-a-uuid", domain.ErrInvalidUserID},
		{domain.PagePosition{ID: rowID}, pagination.Feed, ownerID, domain.ErrInvalidCursor},
		{domain.PagePosition{CreatedAt: time.Now(), ID: "not-a-uuid"}, pagination.Feed, ownerID, domain.ErrInvalidCursor},
		{valid, pagination.Scope("unsupported"), ownerID, domain.ErrInvalidCursor},
	} {
		if _, err := pagination.Encode(test.position, test.scope, test.owner); !errors.Is(err, test.want) {
			t.Fatalf("expected %v, got %v", test.want, err)
		}
	}
	if _, err := pagination.Parse(domain.PageRequest{}, pagination.Feed, "not-a-uuid"); !errors.Is(err, domain.ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}
