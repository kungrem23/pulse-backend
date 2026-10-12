package s3

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/minio/minio-go/v7"
)

var _ domain.PhotoStorage = (*Storage)(nil)

type objectClientStub struct {
	putBucket      string
	putKey         string
	putBody        []byte
	putContentType string
	putErr         error
	removeKey      string
	removeErr      error
	bucketExists   bool
	bucketErr      error
}

func (s *objectClientStub) PutObject(_ context.Context, bucket, key string, reader io.Reader, _ int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	s.putBucket = bucket
	s.putKey = key
	s.putBody, _ = io.ReadAll(reader)
	s.putContentType = opts.ContentType
	return minio.UploadInfo{}, s.putErr
}

func (s *objectClientStub) RemoveObject(_ context.Context, _ string, key string, _ minio.RemoveObjectOptions) error {
	s.removeKey = key
	return s.removeErr
}

func (s *objectClientStub) BucketExists(context.Context, string) (bool, error) {
	return s.bucketExists, s.bucketErr
}

type URLClientStub struct {
	objectURL *url.URL
	err       error
}

func (s *URLClientStub) PresignedGetObject(context.Context, string, string, time.Duration, url.Values) (*url.URL, error) {
	return s.objectURL, s.err
}

func TestStoragePutDeleteAndURL(t *testing.T) {
	objects := &objectClientStub{bucketExists: true}
	objectURL, _ := url.Parse("https://media.example.test/pulse/users/u/photo.png?signature=test")
	storage := newStorage(objects, &URLClientStub{objectURL: objectURL}, "pulse", time.Minute)

	key, err := storage.Put(context.Background(), "users/u/photo.png", []byte("image"), "image/png")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if key != "users/u/photo.png" || objects.putBucket != "pulse" || objects.putKey != key ||
		string(objects.putBody) != "image" || objects.putContentType != "image/png" {
		t.Fatal("put did not forward object data")
	}

	gotURL, err := storage.URL(context.Background(), key)
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	if gotURL != objectURL.String() {
		t.Fatalf("unexpected URL: %s", gotURL)
	}

	if err := storage.Delete(context.Background(), key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if objects.removeKey != key {
		t.Fatalf("unexpected removed key: %s", objects.removeKey)
	}
	if err := storage.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestStorageWrapsProviderFailures(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	objects := &objectClientStub{putErr: providerErr, removeErr: providerErr, bucketErr: providerErr}
	storage := newStorage(objects, &URLClientStub{err: providerErr}, "pulse", time.Minute)

	for name, call := range map[string]func() error{
		"put": func() error {
			_, err := storage.Put(context.Background(), "photo.png", []byte("image"), "image/png")
			return err
		},
		"delete": func() error { return storage.Delete(context.Background(), "photo.png") },
		"url": func() error {
			_, err := storage.URL(context.Background(), "photo.png")
			return err
		},
		"ping": func() error { return storage.Ping(context.Background()) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, providerErr) {
				t.Fatalf("expected wrapped provider error, got %v", err)
			}
		})
	}
}

func TestNewRejectsCredentialBearingEndpointWithoutLeakingIt(t *testing.T) {
	secret := "storage-secret"
	_, err := New(Config{
		Endpoint:       "http://access:" + secret + "@localhost:9000/path",
		PublicEndpoint: "http://localhost:9000",
		Bucket:         "pulse",
		AccessKey:      "access",
		SecretKey:      secret,
		Region:         "us-east-1",
		PresignTTL:     time.Minute,
	})
	if err == nil {
		t.Fatal("expected invalid endpoint to be rejected")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestStorageRejectsInvalidObjectKey(t *testing.T) {
	storage := newStorage(&objectClientStub{}, &URLClientStub{}, "pulse", time.Minute)
	for _, key := range []string{"", "   ", "/absolute.png"} {
		if _, err := storage.Put(context.Background(), key, nil, "image/png"); err == nil {
			t.Fatalf("expected key %q to be rejected", key)
		}
	}
}
