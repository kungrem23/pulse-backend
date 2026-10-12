// Package pgtest creates throwaway PostgreSQL databases for integration tests.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvAdminURL must point at an isolated test server whose role may create
// databases. Tests never touch the database named in the URL itself.
const EnvAdminURL = "PULSE_TEST_DATABASE_URL"

var migrations = []string{"initial_001.sql", "decisions_002.sql"}

// NewDatabase creates a uniquely named database with the MVP schema and drops
// it when the test finishes. The test is skipped when EnvAdminURL is unset.
func NewDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminURL := os.Getenv(EnvAdminURL)
	if adminURL == "" {
		t.Skipf("%s is not set; skipping PostgreSQL integration test", EnvAdminURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer admin.Close(ctx)

	name := "pulse_it_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Errorf("connect to drop test database: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})

	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvAdminURL, err)
	}
	parsed.Path = "/" + name
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, file := range migrations {
		sql, err := os.ReadFile(filepath.Join(migrationsDir(t), file))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
	return pool
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate pgtest source file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("generate database name: %v", err)
	}
	return hex.EncodeToString(buffer)
}
