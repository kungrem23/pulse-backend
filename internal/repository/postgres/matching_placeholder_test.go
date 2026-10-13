package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

// Without any DB connection, unfinished queries must fail explicitly, before
// accessing a pool. Validation remains testable even while SQL is out of scope.
func TestMatchingPlaceholderGuards(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	ctx := context.Background()
	swipes := NewSwipeRepository(nil)
	for _, tc := range []struct {
		from, to string
		want     error
	}{
		{"invalid", b, domain.ErrNotFound}, {a, "invalid", domain.ErrInvalidUserID}, {a, a, domain.ErrSelfSwipe},
	} {
		if match, err := swipes.Save(ctx, tc.from, tc.to, true); match != nil || !errors.Is(err, tc.want) {
			t.Fatalf("unexpected validation: %+v %v", match, err)
		}
	}
	if insertSwipeQuery == "" {
		if match, err := swipes.Save(ctx, a, b, true); match != nil || !errors.Is(err, errQueryNotWritten) {
			t.Fatalf("placeholder must fail safely: %+v %v", match, err)
		}
	}
	matches := NewMatchRepository(nil)
	for _, tc := range []struct {
		owner string
		limit int
		want  error
	}{
		{"invalid", 20, domain.ErrNotFound}, {a, 0, domain.ErrInvalidLimit}, {a, 101, domain.ErrInvalidLimit},
	} {
		if _, err := matches.ListByUserID(ctx, tc.owner, domain.PageQuery{Limit: tc.limit}); !errors.Is(err, tc.want) {
			t.Fatalf("unexpected validation: %v", err)
		}
	}
	if matchesByUserIDQuery == "" {
		if _, err := matches.ListByUserID(ctx, a, domain.PageQuery{Limit: 20}); !errors.Is(err, errQueryNotWritten) {
			t.Fatalf("placeholder must fail safely: %v", err)
		}
	}
}
