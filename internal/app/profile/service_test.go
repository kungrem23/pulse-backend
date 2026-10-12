package profile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

type profileRepositoryStub struct {
	profile        domain.Profile
	profileErr     error
	preferences    domain.Preferences
	preferencesErr error
	saveErr        error
	saved          []domain.Profile

	savedPreferences []domain.Preferences
	sleepCalls       int
}

func (r *profileRepositoryStub) Save(_ context.Context, profile domain.Profile) (domain.Profile, error) {
	r.saved = append(r.saved, profile)
	if r.saveErr != nil {
		return domain.Profile{}, r.saveErr
	}
	profile.IsSleeping = true
	return profile, nil
}

func (r *profileRepositoryStub) ByUserID(context.Context, string) (domain.Profile, error) {
	return r.profile, r.profileErr
}

func (r *profileRepositoryStub) SavePreferences(_ context.Context, preferences domain.Preferences) (domain.Preferences, error) {
	r.savedPreferences = append(r.savedPreferences, preferences)
	return preferences, r.saveErr
}

func (r *profileRepositoryStub) SetSleep(_ context.Context, _ string, isSleeping bool) (bool, error) {
	r.sleepCalls++
	return isSleeping, r.saveErr
}

func (r *profileRepositoryStub) Preferences(context.Context, string) (domain.Preferences, error) {
	return r.preferences, r.preferencesErr
}

func validCommand() UpdateCommand {
	return UpdateCommand{UserID: "user-1", Name: "Анна", Bio: "Люблю горы", Age: 25, Sex: "female", City: "Москва"}
}

func TestUpdateNormalizesAndSaves(t *testing.T) {
	repository := &profileRepositoryStub{}
	command := validCommand()
	command.Name, command.City, command.Bio = "  Анна ", "\tМосква\n", "  "

	profile, err := NewService(repository).Update(context.Background(), command)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	want := domain.Profile{UserID: "user-1", Name: "Анна", Bio: "", Age: 25, Sex: domain.SexFemale, City: "Москва"}
	if len(repository.saved) != 1 || repository.saved[0] != want {
		t.Fatalf("expected %+v to be saved, got %+v", want, repository.saved)
	}
	if !profile.IsSleeping {
		t.Fatal("the stored sleep state must be returned as saved by the repository")
	}
}

func TestUpdateAcceptsBoundaries(t *testing.T) {
	for name, mutate := range map[string]func(*UpdateCommand){
		"name of 100 cyrillic characters": func(c *UpdateCommand) { c.Name = strings.Repeat("я", 100) },
		"city of 100 cyrillic characters": func(c *UpdateCommand) { c.City = strings.Repeat("ё", 100) },
		"bio of 500 cyrillic characters":  func(c *UpdateCommand) { c.Bio = strings.Repeat("ж", 500) },
		"age 18":                          func(c *UpdateCommand) { c.Age = 18 },
		"age 120":                         func(c *UpdateCommand) { c.Age = 120 },
		"male":                            func(c *UpdateCommand) { c.Sex = "male" },
	} {
		t.Run(name, func(t *testing.T) {
			command := validCommand()
			mutate(&command)
			if _, err := NewService(&profileRepositoryStub{}).Update(context.Background(), command); err != nil {
				t.Fatalf("expected success, got %v", err)
			}
		})
	}
}

func TestUpdateRejectsInvalidFieldsBeforeSaving(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UpdateCommand)
		field  string
	}{
		{"empty name", func(c *UpdateCommand) { c.Name = "" }, "name"},
		{"whitespace name", func(c *UpdateCommand) { c.Name = "   " }, "name"},
		{"name of 101 characters", func(c *UpdateCommand) { c.Name = strings.Repeat("я", 101) }, "name"},
		{"bio of 501 characters", func(c *UpdateCommand) { c.Bio = strings.Repeat("ж", 501) }, "bio"},
		{"missing age", func(c *UpdateCommand) { c.Age = 0 }, "age"},
		{"age 17", func(c *UpdateCommand) { c.Age = 17 }, "age"},
		{"age 121", func(c *UpdateCommand) { c.Age = 121 }, "age"},
		{"sex other", func(c *UpdateCommand) { c.Sex = "other" }, "sex"},
		{"sex any is a preference, not a profile sex", func(c *UpdateCommand) { c.Sex = "any" }, "sex"},
		{"sex in another case", func(c *UpdateCommand) { c.Sex = "Female" }, "sex"},
		{"missing city", func(c *UpdateCommand) { c.City = "" }, "city"},
		{"whitespace city", func(c *UpdateCommand) { c.City = " \t " }, "city"},
		{"city of 101 characters", func(c *UpdateCommand) { c.City = strings.Repeat("ё", 101) }, "city"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &profileRepositoryStub{}
			command := validCommand()
			test.mutate(&command)

			_, err := NewService(repository).Update(context.Background(), command)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("expected validation error for %s, got %v", test.field, err)
			}
			if len(repository.saved) != 0 {
				t.Fatal("invalid profile must not reach the repository")
			}
		})
	}
}

func TestUpdateMapsMissingUser(t *testing.T) {
	_, err := NewService(&profileRepositoryStub{saveErr: domain.ErrNotFound}).Update(context.Background(), validCommand())
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestGet(t *testing.T) {
	profile := domain.Profile{UserID: "user-1", Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}

	t.Run("no profile yet", func(t *testing.T) {
		_, err := NewService(&profileRepositoryStub{profileErr: domain.ErrNotFound}).Get(context.Background(), "user-1")
		if !errors.Is(err, ErrProfileNotFound) {
			t.Fatalf("expected ErrProfileNotFound, got %v", err)
		}
	})
	t.Run("no preferences yet", func(t *testing.T) {
		view, err := NewService(&profileRepositoryStub{profile: profile, preferencesErr: domain.ErrNotFound}).Get(context.Background(), "user-1")
		if err != nil || view.Profile != profile || view.Preferences != nil || view.Photos == nil {
			t.Fatalf("unexpected view %+v, %v", view, err)
		}
	})
	t.Run("with preferences", func(t *testing.T) {
		preferences := domain.Preferences{UserID: "user-1", MinAge: 20, MaxAge: 30, Sex: domain.SearchSexAny}
		view, err := NewService(&profileRepositoryStub{profile: profile, preferences: preferences}).Get(context.Background(), "user-1")
		if err != nil || view.Preferences == nil || *view.Preferences != preferences {
			t.Fatalf("unexpected view %+v, %v", view, err)
		}
	})
	t.Run("storage failure", func(t *testing.T) {
		_, err := NewService(&profileRepositoryStub{profile: profile, preferencesErr: errors.New("down")}).Get(context.Background(), "user-1")
		if err == nil || errors.Is(err, ErrProfileNotFound) {
			t.Fatalf("expected an internal error, got %v", err)
		}
	})
}

func TestUpdatePreferences(t *testing.T) {
	valid := PreferencesCommand{UserID: "user-1", MinAge: 18, MaxAge: 120, Sex: "any"}
	repository := &profileRepositoryStub{}
	saved, err := NewService(repository).UpdatePreferences(context.Background(), valid)
	want := domain.Preferences{UserID: "user-1", MinAge: 18, MaxAge: 120, Sex: domain.SearchSexAny}
	if err != nil || saved != want || len(repository.savedPreferences) != 1 {
		t.Fatalf("unexpected result %+v, %v", saved, err)
	}

	for _, ok := range []PreferencesCommand{
		{UserID: "u", MinAge: 25, MaxAge: 25, Sex: "male"},
		{UserID: "u", MinAge: 30, MaxAge: 40, Sex: "female"},
	} {
		if _, err := NewService(&profileRepositoryStub{}).UpdatePreferences(context.Background(), ok); err != nil {
			t.Fatalf("%+v must be accepted, got %v", ok, err)
		}
	}

	tests := []struct {
		name    string
		command PreferencesCommand
		field   string
	}{
		{"missing fields", PreferencesCommand{UserID: "u"}, "min_age"},
		{"min below 18", PreferencesCommand{MinAge: 17, MaxAge: 30, Sex: "any"}, "min_age"},
		{"max above 120", PreferencesCommand{MinAge: 18, MaxAge: 121, Sex: "any"}, "max_age"},
		{"min above max", PreferencesCommand{MinAge: 31, MaxAge: 30, Sex: "any"}, "min_age"},
		{"int16 overflow", PreferencesCommand{MinAge: 18, MaxAge: 65554, Sex: "any"}, "max_age"},
		{"missing sex", PreferencesCommand{MinAge: 18, MaxAge: 30}, "sex"},
		{"sex other", PreferencesCommand{MinAge: 18, MaxAge: 30, Sex: "other"}, "sex"},
		{"sex in another case", PreferencesCommand{MinAge: 18, MaxAge: 30, Sex: "Any"}, "sex"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &profileRepositoryStub{}
			_, err := NewService(repository).UpdatePreferences(context.Background(), test.command)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("expected validation error for %s, got %v", test.field, err)
			}
			if len(repository.savedPreferences) != 0 {
				t.Fatal("invalid preferences must not reach the repository")
			}
		})
	}
}

func TestPreferencesAndSleepRequireProfile(t *testing.T) {
	service := NewService(&profileRepositoryStub{saveErr: domain.ErrNotFound})
	_, err := service.UpdatePreferences(context.Background(), PreferencesCommand{UserID: "u", MinAge: 18, MaxAge: 30, Sex: "any"})
	if !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("expected ErrProfileRequired for preferences, got %v", err)
	}
	if _, err := service.SetSleep(context.Background(), "u", true); !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("expected ErrProfileRequired for sleep, got %v", err)
	}
}

func TestSetSleepReturnsStoredState(t *testing.T) {
	for _, state := range []bool{true, false} {
		stored, err := NewService(&profileRepositoryStub{}).SetSleep(context.Background(), "u", state)
		if err != nil || stored != state {
			t.Fatalf("expected %v, got %v, %v", state, stored, err)
		}
	}
}
