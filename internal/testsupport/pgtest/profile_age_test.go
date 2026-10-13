package pgtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileAgeMigrationPreservesExistingData(t *testing.T) {
	pool := newDatabase(t, migrations[:2])
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO users (id, email, password_hash, age)
VALUES ('11111111-1111-4111-8111-111111111111', 'existing@example.test', 'existing-hash', 25);
INSERT INTO profiles (user_id, name, sex, city)
VALUES ('11111111-1111-4111-8111-111111111111', 'Анна', 'female', 'Москва');
INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
VALUES ('11111111-1111-4111-8111-111111111111', 'existing-session-hash', now() + interval '1 day');
`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var result string
		err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
    'users', (SELECT jsonb_agg(to_jsonb(u)) FROM users u),
    'profiles', (SELECT jsonb_agg(to_jsonb(p)) FROM profiles p),
    'sessions', (SELECT jsonb_agg(to_jsonb(s)) FROM refresh_sessions s)
)::text`).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	sql, err := os.ReadFile(filepath.Join(migrationsDir(t), "profile_age_003.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply profile age migration: %v", err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("migration changed existing data: before %s, after %s", before, after)
	}
	var applied bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 'profile_age_003')").Scan(&applied); err != nil || !applied {
		t.Fatalf("migration version must be recorded: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO users (email, password_hash) VALUES ('new@example.test', 'hash')"); err != nil {
		t.Fatalf("registration without age: %v", err)
	}
	for _, age := range []int{-1, 0, 17, 121} {
		if _, err := pool.Exec(ctx, "UPDATE users SET age = $1 WHERE email = 'new@example.test'", age); err == nil {
			t.Fatalf("age check was lost: accepted %d", age)
		}
	}
	before = snapshot()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, string(sql)); err == nil {
		t.Fatal("reapplying a recorded migration must fail")
	}
	// Exec's failed multi-statement query leaves its transaction open.
	if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	conn.Release()
	if after := snapshot(); after != before {
		t.Fatal("reapplying the migration must preserve all data")
	}
}
