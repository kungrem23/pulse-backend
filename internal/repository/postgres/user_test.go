package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

func TestUserRepositoryCreateAndFind(t *testing.T) {
	repository := postgres.NewUserRepository(pgtest.NewDatabase(t))
	ctx := context.Background()

	created, err := repository.Create(ctx, domain.User{Email: "user@example.com", PasswordHash: "hash", Age: 18})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() || created.IsBlocked {
		t.Fatalf("database defaults were not returned: %+v", created)
	}

	byEmail, err := repository.ByEmail(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	byID, err := repository.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	for _, found := range []domain.User{byEmail, byID} {
		if found.ID != created.ID || found.Email != "user@example.com" ||
			found.PasswordHash != "hash" || found.Age != 18 || !found.CreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("unexpected user: %+v", found)
		}
	}
}

func TestUserRepositoryNotFound(t *testing.T) {
	repository := postgres.NewUserRepository(pgtest.NewDatabase(t))
	ctx := context.Background()

	if _, err := repository.ByEmail(ctx, "missing@example.com"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown email, got %v", err)
	}
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		if _, err := repository.ByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for id %q, got %v", id, err)
		}
	}
}

func TestUserRepositoryRejectsInvalidRows(t *testing.T) {
	repository := postgres.NewUserRepository(pgtest.NewDatabase(t))
	ctx := context.Background()

	for _, user := range []domain.User{
		{Email: "young@example.com", PasswordHash: "hash", Age: 17},
		{Email: "old@example.com", PasswordHash: "hash", Age: 121},
		{Email: "Upper@Example.com", PasswordHash: "hash", Age: 30},
	} {
		if _, err := repository.Create(ctx, user); err == nil || errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected a constraint error for %+v, got %v", user, err)
		}
	}
}

func TestUserRepositoryConcurrentDuplicateEmail(t *testing.T) {
	repository := postgres.NewUserRepository(pgtest.NewDatabase(t))
	const attempts = 8

	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repository.Create(context.Background(), domain.User{
				Email: "same@example.com", PasswordHash: "hash", Age: 25,
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	created, conflicts := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || conflicts != attempts-1 {
		t.Fatalf("expected 1 success and %d conflicts, got %d and %d", attempts-1, created, conflicts)
	}
}
