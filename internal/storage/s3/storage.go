package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxPresignTTL = 7 * 24 * time.Hour

type Config struct {
	Endpoint       string
	PublicEndpoint string
	Bucket         string
	AccessKey      string
	SecretKey      string
	Region         string
	PresignTTL     time.Duration
}

type objectClient interface {
	PutObject(context.Context, string, string, io.Reader, int64, minio.PutObjectOptions) (minio.UploadInfo, error)
	RemoveObject(context.Context, string, string, minio.RemoveObjectOptions) error
	BucketExists(context.Context, string) (bool, error)
}

type urlClient interface {
	PresignedGetObject(context.Context, string, string, time.Duration, url.Values) (*url.URL, error)
}

// Storage implements domain.PhotoStorage using an S3-compatible API. The
// internal endpoint is used for object operations, while PublicEndpoint is
// used to produce links that can be opened by the frontend.
type Storage struct {
	objects    objectClient
	urls       urlClient
	bucket     string
	presignTTL time.Duration
}

func New(cfg Config) (*Storage, error) {
	endpoint, secure, err := parseEndpoint("S3_ENDPOINT", cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	publicEndpoint, publicSecure, err := parseEndpoint("S3_PUBLIC_ENDPOINT", cfg.PublicEndpoint)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("S3_BUCKET must not be empty")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("S3 credentials must not be empty")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return nil, fmt.Errorf("S3_REGION must not be empty")
	}
	if cfg.PresignTTL <= 0 || cfg.PresignTTL > maxPresignTTL {
		return nil, fmt.Errorf("S3_PRESIGN_TTL must be between 1ns and 168h")
	}

	creds := credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	objects, err := minio.New(endpoint, &minio.Options{
		Creds:  creds,
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	urls, err := minio.New(publicEndpoint, &minio.Options{
		Creds:  creds,
		Secure: publicSecure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create public S3 client: %w", err)
	}
	return newStorage(objects, urls, cfg.Bucket, cfg.PresignTTL), nil
}

func newStorage(objects objectClient, urls urlClient, bucket string, presignTTL time.Duration) *Storage {
	return &Storage{objects: objects, urls: urls, bucket: bucket, presignTTL: presignTTL}
}

func (s *Storage) Ping(ctx context.Context) error {
	exists, err := s.objects.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check photo bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("photo bucket does not exist")
	}
	return nil
}

func (s *Storage) Put(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	if strings.TrimSpace(contentType) == "" {
		return "", fmt.Errorf("content type must not be empty")
	}
	_, err := s.objects.PutObject(
		ctx,
		s.bucket,
		key,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return "", fmt.Errorf("store photo object: %w", err)
	}
	return key, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if err := s.objects.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete photo object: %w", err)
	}
	return nil
}

func (s *Storage) URL(ctx context.Context, key string) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	objectURL, err := s.urls.PresignedGetObject(ctx, s.bucket, key, s.presignTTL, nil)
	if err != nil {
		return "", fmt.Errorf("create photo URL: %w", err)
	}
	return objectURL.String(), nil
}

func parseEndpoint(name, value string) (string, bool, error) {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("%s must be an HTTP(S) origin without credentials, path, query or fragment", name)
	}
	return parsed.Host, parsed.Scheme == "https", nil
}

func validateObjectKey(key string) error {
	if strings.TrimSpace(key) == "" || strings.HasPrefix(key, "/") {
		return fmt.Errorf("object key must be non-empty and relative")
	}
	return nil
}
