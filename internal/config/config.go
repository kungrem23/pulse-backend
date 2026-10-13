package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

const defaultJWTSecret = "change-me-in-production-at-least-32-bytes"

type Config struct {
	Environment       string
	HTTPAddr          string
	DatabaseURL       string
	JWTSecret         string
	JWTIssuer         string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	PhotoStorage      PhotoStorageConfig
	CORSOrigins       []string
	AuthRateLimit     middleware.RateLimit
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type PhotoStorageConfig struct {
	Endpoint       string
	PublicEndpoint string
	Bucket         string
	AccessKey      string
	SecretKey      string
	Region         string
	PresignTTL     time.Duration
}

func Load() (Config, error) {
	storageEndpoint := env("S3_ENDPOINT", "http://localhost:9000")
	cfg := Config{
		Environment: env("APP_ENV", "local"),
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		JWTSecret:   env("JWT_SECRET", defaultJWTSecret),
		JWTIssuer:   env("JWT_ISSUER", "pulse"),
		PhotoStorage: PhotoStorageConfig{
			Endpoint:       storageEndpoint,
			PublicEndpoint: env("S3_PUBLIC_ENDPOINT", storageEndpoint),
			Bucket:         env("S3_BUCKET", "pulse"),
			AccessKey:      env("S3_ACCESS_KEY", "pulse"),
			SecretKey:      env("S3_SECRET_KEY", "pulse-minio-secret"),
			Region:         env("S3_REGION", "us-east-1"),
		},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var err error
	cfg.CORSOrigins, err = middleware.ParseOrigins(env("CORS_ALLOWED_ORIGINS", ""))
	if err != nil {
		// ParseOrigins includes the input in its error; do not expose an origin
		// containing accidentally supplied credentials in a startup log.
		return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain exact HTTP(S) origins without credentials, paths or wildcards")
	}
	// Technical defaults, not product thresholds: each auth route/IP gets its
	// own budget. MaxKeys bounds memory; these are overridable via environment.
	cfg.AuthRateLimit.Requests, err = strconv.Atoi(env("AUTH_RATE_LIMIT_REQUESTS", "30"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT_REQUESTS must be a positive integer")
	}
	cfg.AuthRateLimit.Window, err = time.ParseDuration(env("AUTH_RATE_LIMIT_WINDOW", "1m"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT_WINDOW must be a duration of at least 1s")
	}
	cfg.AuthRateLimit.MaxKeys, err = strconv.Atoi(env("AUTH_RATE_LIMIT_MAX_KEYS", "100000"))
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT_MAX_KEYS must be a non-negative integer")
	}
	if err := cfg.AuthRateLimit.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid auth rate limit configuration: %w", err)
	}
	cfg.DatabaseURL, err = databaseURL()
	if err != nil {
		return Config{}, err
	}
	cfg.ShutdownTimeout, err = time.ParseDuration(env("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg.AccessTokenTTL, err = time.ParseDuration(env("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("parse JWT_ACCESS_TTL: %w", err)
	}
	cfg.RefreshTokenTTL, err = time.ParseDuration(env("JWT_REFRESH_TTL", "720h"))
	if err != nil {
		return Config{}, fmt.Errorf("parse JWT_REFRESH_TTL: %w", err)
	}
	cfg.PhotoStorage.PresignTTL, err = time.ParseDuration(env("S3_PRESIGN_TTL", "15m"))
	if err != nil || cfg.PhotoStorage.PresignTTL <= 0 || cfg.PhotoStorage.PresignTTL > 7*24*time.Hour {
		return Config{}, fmt.Errorf("S3_PRESIGN_TTL must be a duration between 1ns and 168h")
	}
	if err := validatePhotoStorage(cfg.PhotoStorage); err != nil {
		return Config{}, err
	}
	if cfg.Environment == "production" && cfg.JWTSecret == defaultJWTSecret {
		return Config{}, fmt.Errorf("JWT_SECRET must be configured in production")
	}
	return cfg, nil
}

func validatePhotoStorage(cfg PhotoStorageConfig) error {
	for name, value := range map[string]string{
		"S3_BUCKET":     cfg.Bucket,
		"S3_ACCESS_KEY": cfg.AccessKey,
		"S3_SECRET_KEY": cfg.SecretKey,
		"S3_REGION":     cfg.Region,
	} {
		if value == "" {
			return fmt.Errorf("%s must not be empty", name)
		}
	}
	for name, value := range map[string]string{
		"S3_ENDPOINT":        cfg.Endpoint,
		"S3_PUBLIC_ENDPOINT": cfg.PublicEndpoint,
	} {
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("%s must be an HTTP(S) origin without credentials, path, query or fragment", name)
		}
	}
	return nil
}

// Database credentials are configured as separate fields. url.URL safely
// escapes credentials, including passwords containing @, /, or :.
func databaseURL() (string, error) {
	port := env("DB_PORT", "5432")
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return "", fmt.Errorf("DB_PORT must be between 1 and 65535")
	}
	connection := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(env("DB_USER", "pulse"), env("DB_PASSWORD", "pulse")),
		Host:   net.JoinHostPort(env("DB_HOST", "localhost"), port),
		Path:   "/" + env("DB_NAME", "pulse"),
	}
	query := url.Values{}
	query.Set("sslmode", env("DB_SSLMODE", "disable"))
	connection.RawQuery = query.Encode()
	return connection.String(), nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
