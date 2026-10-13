package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

// These tests use a throwaway database configured by PULSE_TEST_DATABASE_URL.
func TestPhotoRepositoryOwnershipAndPositions(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx := context.Background()
	users := postgres.NewUserRepository(pool)
	profiles := postgres.NewProfileRepository(pool)
	photos := postgres.NewPhotoRepository(pool)
	createUser := func(email string) string {
		t.Helper()
		user, err := users.Create(ctx, domain.User{Email: email, PasswordHash: "hash"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := profiles.Save(ctx, domain.Profile{UserID: user.ID, Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}); err != nil {
			t.Fatal(err)
		}
		return user.ID
	}
	owner, other := createUser("a@example.test"), createUser("b@example.test")
	newPhoto := func(position int16) domain.Photo {
		id := uuid.NewString()
		return domain.Photo{ID: id, UserID: owner, ObjectKey: "test/" + id, Position: position}
	}
	first := newPhoto(1)
	if err := photos.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if saved, err := photos.ByIDForUser(ctx, first.ID, owner); err != nil || saved != first {
		t.Fatalf("photo round-trip: %+v, %v", saved, err)
	}
	if _, err := photos.ByIDForUser(ctx, first.ID, other); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign lookup must return ErrNotFound: %v", err)
	}
	if err := photos.Delete(ctx, first.ID, other); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign deletion must return ErrNotFound: %v", err)
	}
	if err := photos.Create(ctx, newPhoto(1)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate position must return ErrConflict: %v", err)
	}
	for position := int16(5); position >= 2; position-- {
		if err := photos.Create(ctx, newPhoto(position)); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := photos.ByUserID(ctx, owner)
	if err != nil || len(listed) != 5 {
		t.Fatalf("five stored photos: %+v, %v", listed, err)
	}
	for i, photo := range listed {
		if photo.Position != int16(i+1) || photo.UserID != owner {
			t.Fatalf("photo list must be owned and ordered by position: %+v", listed)
		}
	}
	if err := photos.Create(ctx, newPhoto(6)); err == nil {
		t.Fatal("sixth position must violate database constraints")
	}
	if otherPhotos, err := photos.ByUserID(ctx, other); err != nil || otherPhotos == nil || len(otherPhotos) != 0 {
		t.Fatalf("no photos must be a non-nil empty slice: %+v, %v", otherPhotos, err)
	}
	orphan := newPhoto(1)
	orphan.UserID = uuid.NewString()
	if err := photos.Create(ctx, orphan); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing profile FK must return ErrNotFound: %v", err)
	}
	for _, photo := range listed {
		if err := photos.Delete(ctx, photo.ID, owner); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := profiles.ByUserID(ctx, owner); err != nil {
		t.Fatalf("last photo removal must preserve profile: %v", err)
	}
	if err := photos.Delete(ctx, first.ID, owner); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("repeated metadata deletion must return ErrNotFound: %v", err)
	}
}

func TestPhotoRepositoryConcurrentPosition(t *testing.T) {
	pool := pgtest.NewDatabase(t)
	ctx := context.Background()
	user, err := postgres.NewUserRepository(pool).Create(ctx, domain.User{Email: "a@example.test", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewProfileRepository(pool).Save(ctx, domain.Profile{UserID: user.ID, Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}); err != nil {
		t.Fatal(err)
	}
	photos := postgres.NewPhotoRepository(pool)
	const attempts = 8
	var wg sync.WaitGroup
	errorsChannel := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := uuid.NewString()
			errorsChannel <- photos.Create(ctx, domain.Photo{ID: id, UserID: user.ID, ObjectKey: "test/" + id, Position: 1})
		}()
	}
	wg.Wait()
	close(errorsChannel)
	created := 0
	for err := range errorsChannel {
		if err == nil {
			created++
		} else if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("unexpected concurrent insertion error: %v", err)
		}
	}
	listed, err := photos.ByUserID(ctx, user.ID)
	if err != nil || created != 1 || len(listed) != 1 {
		t.Fatalf("one position must have exactly one row: %d %+v %v", created, listed, err)
	}
}
