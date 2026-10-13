package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

// Expiration must be checked after acquiring the lock, not before waiting for it.
func TestRefreshSessionExpiresWhileWaitingForLock(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	user, err := postgres.NewUserRepository(pool).Create(ctx, domain.User{
		Email: "refresh-wait@example.test", PasswordHash: "hash", Age: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
		VALUES ($1, 'old-test-hash', clock_timestamp() + interval '2 seconds')`, user.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "SELECT id FROM refresh_sessions WHERE token_hash = 'old-test-hash' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- postgres.NewRefreshSessionRepository(pool).Rotate(ctx, "old-test-hash", domain.RefreshSession{
			UserID: user.ID, TokenHash: "next-test-hash", ExpiresAt: time.Now().Add(time.Hour),
		})
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("rotation must wait for the row lock, got %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	for {
		var expired bool
		if err := pool.QueryRow(ctx, `SELECT expires_at <= clock_timestamp()
			FROM refresh_sessions WHERE token_hash = 'old-test-hash'`).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expired session must not rotate after lock wait: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM refresh_sessions WHERE token_hash = 'next-test-hash'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rotation of an expired session created a replacement")
	}
}
