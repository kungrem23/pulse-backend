package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

func TestProfileSaveIsAtomic(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx := context.Background()
	user, err := postgres.NewUserRepository(pool).Create(ctx, domain.User{Email: "a@example.com", PasswordHash: "hash", Age: 20})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	profiles := postgres.NewProfileRepository(pool)

	// Bypasses app validation so that the profile write fails after users.age was updated.
	_, err = profiles.Save(ctx, domain.Profile{
		UserID: user.ID, Name: "Анна", Bio: strings.Repeat("ж", 501), Age: 30, Sex: domain.SexFemale, City: "Москва",
	})
	if err == nil {
		t.Fatal("expected the bio constraint to fail")
	}
	stored, err := postgres.NewUserRepository(pool).ByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Age != 20 {
		t.Fatalf("a failed profile write must roll back users.age, got %d", stored.Age)
	}
	if _, err := profiles.ByUserID(ctx, user.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no profile must exist after the failed save, got %v", err)
	}
}

func TestProfileSaveForMissingUser(t *testing.T) {
	profiles := postgres.NewProfileRepository(pgtest.NewDatabase(t))
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		_, err := profiles.Save(context.Background(), domain.Profile{
			UserID: id, Name: "Анна", Age: 30, Sex: domain.SexFemale, City: "Москва",
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s: expected ErrNotFound, got %v", id, err)
		}
	}
}
