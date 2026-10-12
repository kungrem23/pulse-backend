//go:build integration

package s3

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStorageIntegration(t *testing.T) {
	storage, err := New(Config{
		Endpoint:       requiredEnv(t, "S3_ENDPOINT"),
		PublicEndpoint: requiredEnv(t, "S3_PUBLIC_ENDPOINT"),
		Bucket:         requiredEnv(t, "S3_BUCKET"),
		AccessKey:      requiredEnv(t, "S3_ACCESS_KEY"),
		SecretKey:      requiredEnv(t, "S3_SECRET_KEY"),
		Region:         "us-east-1",
		PresignTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := storage.Ping(ctx); err != nil {
		t.Fatalf("ping storage: %v", err)
	}

	key := "integration/" + time.Now().UTC().Format("20060102T150405.000000000") + ".png"
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if _, err := storage.Put(ctx, key, image, "image/png"); err != nil {
		t.Fatalf("put image: %v", err)
	}
	t.Cleanup(func() { _ = storage.Delete(context.Background(), key) })

	objectURL, err := storage.URL(ctx, key)
	if err != nil {
		t.Fatalf("create URL: %v", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(objectURL) // #nosec G107 -- isolated test endpoint
	if err != nil {
		t.Fatalf("download image: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read image: %v", readErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != string(image) || response.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected download: status=%d content-type=%q body-size=%d", response.StatusCode, response.Header.Get("Content-Type"), len(body))
	}

	if err := storage.Delete(ctx, key); err != nil {
		t.Fatalf("delete image: %v", err)
	}
	response, err = client.Get(objectURL) // #nosec G107 -- isolated test endpoint
	if err != nil {
		t.Fatalf("request deleted image: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted object returned status %d", response.StatusCode)
	}
}

func TestStorageIntegrationInvalidCredentialsDoNotLeak(t *testing.T) {
	secret := "integration-secret-that-must-not-leak"
	storage, err := New(Config{
		Endpoint:       requiredEnv(t, "S3_ENDPOINT"),
		PublicEndpoint: requiredEnv(t, "S3_PUBLIC_ENDPOINT"),
		Bucket:         requiredEnv(t, "S3_BUCKET"),
		AccessKey:      requiredEnv(t, "S3_ACCESS_KEY"),
		SecretKey:      secret,
		Region:         "us-east-1",
		PresignTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = storage.Ping(ctx)
	if err == nil {
		t.Fatal("expected invalid credentials to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("storage error leaked secret: %v", err)
	}
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
