//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	photostorage "github.com/kungrem23/pulse-backend/internal/storage/s3"
)

// Track only keys created by this test. Cleanup never lists/deletes a bucket or
// touches preexisting objects; PostgreSQL databases are isolated by pgtest.
type trackedPhotoStorage struct {
	domain.PhotoStorage
	mu   sync.Mutex
	keys []string
}

func (s *trackedPhotoStorage) Put(ctx context.Context, key string, content []byte, contentType string) (string, error) {
	stored, err := s.PhotoStorage.Put(ctx, key, content, contentType)
	if err == nil {
		s.mu.Lock()
		s.keys = append(s.keys, stored)
		s.mu.Unlock()
	}
	return stored, err
}

func newS3IntegrationServer(t *testing.T) *integrationServer {
	t.Helper()
	for _, name := range []string{"PULSE_TEST_DATABASE_URL", "S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		if os.Getenv(name) == "" {
			t.Skipf("%s is not set; configure isolated PostgreSQL/MinIO for integration tests", name)
		}
	}
	server := newIntegrationServer(t)
	publicEndpoint := os.Getenv("S3_PUBLIC_ENDPOINT")
	if publicEndpoint == "" {
		publicEndpoint = os.Getenv("S3_ENDPOINT")
	}
	store, err := photostorage.New(photostorage.Config{
		Endpoint: os.Getenv("S3_ENDPOINT"), PublicEndpoint: publicEndpoint,
		Bucket: os.Getenv("S3_BUCKET"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"),
		Region: "us-east-1", PresignTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	tracked := &trackedPhotoStorage{PhotoStorage: store}
	server.storage = tracked
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, key := range tracked.keys {
			if err := store.Delete(ctx, key); err != nil {
				t.Errorf("clean up T08 test object: %v", err)
			}
		}
	})
	return server
}

func TestPhotosWithPostgresAndS3(t *testing.T) {
	server := newS3IntegrationServer(t)
	tracked := server.storage.(*trackedPhotoStorage)
	expect := func(response *httptest.ResponseRecorder, code int) {
		t.Helper()
		if response.Code != code {
			t.Fatalf("expected %d, got %d: %s", code, response.Code, response.Body)
		}
	}
	register := func(email string) (string, string) {
		t.Helper()
		response := server.do(t, "/api/v1/auth/register", `{"email":"`+email+`","password":"password123"}`, "")
		expect(response, 201)
		var body struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.User.ID, decodeSession(t, response).AccessToken
	}
	owner, token := register("owner@example.test")
	_, foreignToken := register("other@example.test")
	content := photoPNG(t)
	upload := func(position int) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.router(t).ServeHTTP(response, photoRequest(t, content, position, token))
		return response
	}
	expect(upload(1), 409) // No profile: uploaded object is compensated.
	expect(server.put(t, "/api/v1/profile", `{"name":"Анна","age":25,"sex":"female","city":"Москва"}`, token), 200)
	const attempts = 8
	var wg sync.WaitGroup
	statuses := make(chan int, attempts)
	for range attempts {
		wg.Add(1)
		go func() { defer wg.Done(); statuses <- upload(1).Code }()
	}
	wg.Wait()
	close(statuses)
	created := 0
	for status := range statuses {
		if status == 201 {
			created++
		} else if status != 409 {
			t.Fatalf("concurrent upload returned %d", status)
		}
	}
	if created != 1 {
		t.Fatalf("expected one successful concurrent upload, got %d", created)
	}
	for position := 2; position <= domain.MaxPhotos; position++ {
		expect(upload(position), 201)
	}
	expect(upload(5), 409)
	expect(upload(6), 422)
	response := server.get(t, "/api/v1/profile", token)
	expect(response, 200)
	var view struct {
		Photos []struct {
			ID, URL  string
			Position int16
		} `json:"photos"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || len(view.Photos) != 5 {
		t.Fatalf("expected five public photos: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	getURL := func(url string, want int) {
		t.Helper()
		response, err := client.Get(url)
		if err != nil {
			t.Fatalf("frontend photo GET: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("frontend photo GET: expected %d, got %d", want, response.StatusCode)
		}
		if want == 200 {
			body, err := io.ReadAll(io.LimitReader(response.Body, domain.MaxPhotoBytes+1))
			if err != nil || !bytes.Equal(body, content) {
				t.Fatal("frontend URL did not return the uploaded PNG")
			}
		}
	}
	for i, photo := range view.Photos {
		if photo.Position != int16(i+1) {
			t.Fatal("profile photo order must be position ASC")
		}
		getURL(photo.URL, 200)
		expect(server.request(t, http.MethodDelete, "/api/v1/photos/"+photo.ID, "", foreignToken), 404)
		getURL(photo.URL, 200)
		expect(server.request(t, http.MethodDelete, "/api/v1/photos/"+photo.ID, "", token), 204)
		getURL(photo.URL, 404)
		expect(server.request(t, http.MethodDelete, "/api/v1/photos/"+photo.ID, "", token), 404)
	}
	listed, err := postgres.NewPhotoRepository(server.pool).ByUserID(context.Background(), owner)
	if err != nil || len(listed) != 0 {
		t.Fatalf("all photo metadata must be removed: %v", err)
	}
	response = server.get(t, "/api/v1/profile", token)
	expect(response, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.Photos == nil || len(view.Photos) != 0 {
		t.Fatal("profile must remain with photos: []")
	}
	// Every created object, including failed concurrent/missing-profile uploads,
	// must now be gone from real storage, not just from the metadata projection.
	for _, key := range tracked.keys {
		url, err := tracked.URL(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		getURL(url, 404)
	}
}
