package config

import (
	"net/url"
	"testing"
)

func TestLoadBuildsDatabaseURLFromSeparateFields(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "pulse")
	t.Setenv("DB_PASSWORD", "p@ss/word:with?symbols#")
	t.Setenv("DB_NAME", "pulse-test")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("APP_ENV", "local")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("parse connection URL: %v", err)
	}
	password, _ := parsed.User.Password()
	if password != "p@ss/word:with?symbols#" || parsed.User.Username() != "pulse" {
		t.Fatal("database credentials were not preserved")
	}
	if parsed.Host != "localhost:5432" || parsed.Path != "/pulse-test" || parsed.Query().Get("sslmode") != "require" {
		t.Fatal("database connection fields were not preserved")
	}
}

func TestLoadRejectsInvalidDatabasePort(t *testing.T) {
	for _, port := range []string{"not-a-number", "0", "65536"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("DB_PORT", port)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid DB_PORT to be rejected")
			}
		})
	}
}
