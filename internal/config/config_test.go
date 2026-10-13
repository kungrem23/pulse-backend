package config

import (
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestLoadReadsPhotoStorageSettings(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "http://minio:9000")
	t.Setenv("S3_PUBLIC_ENDPOINT", "https://media.example.test")
	t.Setenv("S3_BUCKET", "pulse-media")
	t.Setenv("S3_ACCESS_KEY", "access")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("S3_REGION", "eu-central-1")
	t.Setenv("S3_PRESIGN_TTL", "20m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PhotoStorage.Endpoint != "http://minio:9000" ||
		cfg.PhotoStorage.PublicEndpoint != "https://media.example.test" ||
		cfg.PhotoStorage.Bucket != "pulse-media" ||
		cfg.PhotoStorage.Region != "eu-central-1" ||
		cfg.PhotoStorage.PresignTTL != 20*time.Minute {
		t.Fatalf("unexpected photo storage config: %#v", cfg.PhotoStorage)
	}
}

func TestLoadRejectsInvalidPhotoStorageSettingsWithoutLeakingSecret(t *testing.T) {
	secret := "must-not-appear-in-errors"
	t.Setenv("S3_SECRET_KEY", secret)
	t.Setenv("S3_PUBLIC_ENDPOINT", "https://user:password@example.test/path")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid S3_PUBLIC_ENDPOINT to be rejected")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "password") {
		t.Fatalf("configuration error leaked credentials: %v", err)
	}
}

func TestLoadRejectsInvalidPresignTTL(t *testing.T) {
	for _, value := range []string{"not-a-duration", "0s", "169h"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("S3_PRESIGN_TTL", value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid S3_PRESIGN_TTL to be rejected")
			}
		})
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

func TestLoadRejectsInvalidTokenTTL(t *testing.T) {
	for _, key := range []string{"JWT_ACCESS_TTL", "JWT_REFRESH_TTL"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "soon")
			if _, err := Load(); err == nil {
				t.Fatalf("expected invalid %s to be rejected", key)
			}
		})
	}
}

func TestLoadHTTPPolicyDefaultsAndOverrides(t *testing.T) {
	for _, name := range []string{"CORS_ALLOWED_ORIGINS", "AUTH_RATE_LIMIT_REQUESTS", "AUTH_RATE_LIMIT_WINDOW", "AUTH_RATE_LIMIT_MAX_KEYS"} {
		t.Setenv(name, "")
	}
	cfg, err := Load()
	if err != nil || len(cfg.CORSOrigins) != 0 || cfg.AuthRateLimit.Requests != 30 || cfg.AuthRateLimit.Window != time.Minute || cfg.AuthRateLimit.MaxKeys != 100000 {
		t.Fatalf("HTTP policy defaults: %v", err)
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://frontend.example,http://localhost:5173,https://frontend.example")
	t.Setenv("AUTH_RATE_LIMIT_REQUESTS", "7")
	t.Setenv("AUTH_RATE_LIMIT_WINDOW", "2m")
	t.Setenv("AUTH_RATE_LIMIT_MAX_KEYS", "200")
	cfg, err = Load()
	if err != nil || len(cfg.CORSOrigins) != 2 || cfg.AuthRateLimit.Requests != 7 || cfg.AuthRateLimit.Window != 2*time.Minute || cfg.AuthRateLimit.MaxKeys != 200 {
		t.Fatalf("HTTP policy overrides: %v", err)
	}
}

func TestLoadRejectsInvalidHTTPPolicy(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"CORS_ALLOWED_ORIGINS", "*"}, {"CORS_ALLOWED_ORIGINS", "https://frontend.example/path"},
		{"CORS_ALLOWED_ORIGINS", "https://user:DO-NOT-LOG-THIS@example.test"},
		{"AUTH_RATE_LIMIT_REQUESTS", "0"}, {"AUTH_RATE_LIMIT_REQUESTS", "-1"}, {"AUTH_RATE_LIMIT_REQUESTS", "abc"},
		{"AUTH_RATE_LIMIT_WINDOW", "0s"}, {"AUTH_RATE_LIMIT_WINDOW", "500ms"}, {"AUTH_RATE_LIMIT_WINDOW", "abc"},
		{"AUTH_RATE_LIMIT_MAX_KEYS", "-1"}, {"AUTH_RATE_LIMIT_MAX_KEYS", "abc"},
	} {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			_, err := Load()
			if err == nil || strings.Contains(err.Error(), "DO-NOT-LOG-THIS") {
				t.Fatalf("invalid policy must be rejected without leaking credentials: %v", err)
			}
		})
	}
}
