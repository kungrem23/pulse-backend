package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

const defaultJWTSecret = "change-me-in-production-at-least-32-bytes"

type Config struct {
	Environment       string
	HTTPAddr          string
	DatabaseURL       string
	JWTSecret         string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Environment:       env("APP_ENV", "local"),
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		JWTSecret:         env("JWT_SECRET", defaultJWTSecret),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var err error
	cfg.DatabaseURL, err = databaseURL()
	if err != nil {
		return Config{}, err
	}
	cfg.ShutdownTimeout, err = time.ParseDuration(env("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
	}
	if cfg.Environment == "production" && cfg.JWTSecret == defaultJWTSecret {
		return Config{}, fmt.Errorf("JWT_SECRET must be configured in production")
	}
	return cfg, nil
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
