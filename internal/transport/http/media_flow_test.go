package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	appprofile "github.com/kungrem23/pulse-backend/internal/app/profile"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

const foreignPhotoOwner = "22222222-2222-4222-8222-222222222222"

// These thread-safe test adapters exercise real router/app code while SQL is
// intentionally unfinished. They do not replace PostgreSQL/MinIO acceptance.
type photoFlowRepository struct {
	mu        sync.Mutex
	photos    map[string]domain.Photo
	createErr error
	deleteErr error
}

func (r *photoFlowRepository) Create(_ context.Context, photo domain.Photo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	for _, existing := range r.photos {
		if existing.UserID == photo.UserID && existing.Position == photo.Position {
			return domain.ErrConflict
		}
	}
	r.photos[photo.ID] = photo
	return nil
}

func (r *photoFlowRepository) ByIDForUser(_ context.Context, id, owner string) (domain.Photo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	photo, ok := r.photos[id]
	if !ok || photo.UserID != owner {
		return domain.Photo{}, domain.ErrNotFound
	}
	return photo, nil
}

func (r *photoFlowRepository) ByUserID(_ context.Context, owner string) ([]domain.Photo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	photos := make([]domain.Photo, 0)
	for position := int16(1); position <= domain.MaxPhotos; position++ {
		for _, photo := range r.photos {
			if photo.UserID == owner && photo.Position == position {
				photos = append(photos, photo)
			}
		}
	}
	return photos, nil
}

func (r *photoFlowRepository) Delete(_ context.Context, id, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteErr != nil {
		return r.deleteErr
	}
	photo, ok := r.photos[id]
	if !ok || photo.UserID != owner {
		return domain.ErrNotFound
	}
	delete(r.photos, id)
	return nil
}

type photoFlowStorage struct {
	mu        sync.Mutex
	objects   map[string][]byte
	putErr    error
	deleteErr error
}

func (s *photoFlowStorage) Put(_ context.Context, key string, data []byte, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return "", s.putErr
	}
	s.objects[key] = append([]byte(nil), data...)
	return key, nil
}

func (s *photoFlowStorage) URL(_ context.Context, key string) (string, error) {
	return "https://media.example.test/" + path.Base(key), nil
}

func (s *photoFlowStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, key)
	return nil
}

type photoFlowProfiles struct{ domain.ProfileRepository }

func (photoFlowProfiles) ByUserID(_ context.Context, owner string) (domain.Profile, error) {
	return domain.Profile{UserID: owner, Name: "Анна", Age: 25, Sex: domain.SexFemale, City: "Москва"}, nil
}

func (photoFlowProfiles) Preferences(context.Context, string) (domain.Preferences, error) {
	return domain.Preferences{}, domain.ErrNotFound
}

type photoFlowAuth struct{ appauth.Service }

func (photoFlowAuth) Authenticate(_ context.Context, token string) (string, error) {
	if token == "foreign" {
		return foreignPhotoOwner, nil
	}
	if token == "valid" {
		return sampleID, nil
	}
	return "", appauth.ErrInvalidAccessToken
}

func photoRequest(t *testing.T, content []byte, position int, token string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "content-not-extension.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("position", strconv.Itoa(position)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/photos", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func photoPNG(t *testing.T) []byte {
	t.Helper()
	var content bytes.Buffer
	if err := png.Encode(&content, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func TestPhotoHTTPFlowAndConcurrentPositions(t *testing.T) {
	photos := &photoFlowRepository{photos: make(map[string]domain.Photo)}
	storage := &photoFlowStorage{objects: make(map[string][]byte)}
	router := NewRouter(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: photoFlowAuth{},
		Media: appmedia.NewService(photos, storage), Profile: appprofile.NewService(photoFlowProfiles{}, photos, storage),
	})
	request := func(req *http.Request, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("expected %d, got %d %s", want, response.Code, response.Body)
		}
		return response
	}
	getProfile := func(token string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}
	content := photoPNG(t)
	const attempts = 8
	var wg sync.WaitGroup
	statuses := make(chan int, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			router.ServeHTTP(response, photoRequest(t, content, 1, "valid"))
			statuses <- response.Code
		}()
	}
	wg.Wait()
	close(statuses)
	created := 0
	for status := range statuses {
		if status == 201 {
			created++
		} else if status != 409 {
			t.Fatalf("unexpected concurrent upload status: %d", status)
		}
	}
	if created != 1 || len(photos.photos) != 1 || len(storage.objects) != 1 {
		t.Fatalf("one position must have one metadata row and one object: %d, %d, %d", created, len(photos.photos), len(storage.objects))
	}
	for position := 2; position <= domain.MaxPhotos; position++ {
		request(photoRequest(t, content, position, "valid"), 201)
	}
	request(photoRequest(t, content, 5, "valid"), 409)
	request(photoRequest(t, content, 6, "valid"), 422)
	request(photoRequest(t, []byte("fake PNG"), 1, "valid"), 415)
	if len(photos.photos) != 5 || len(storage.objects) != 5 {
		t.Fatal("rejected uploads must leave exactly five photos/objects")
	}
	response := request(getProfile("valid"), 200)
	if strings.Count(response.Body.String(), `"url":`) != 5 || strings.Contains(response.Body.String(), "object_key") {
		t.Fatalf("profile must show public URLs for five photos: %s", response.Body)
	}
	foreign := request(getProfile("foreign"), 200)
	if !strings.Contains(foreign.Body.String(), `"photos":[]`) {
		t.Fatalf("another user sees own photos only: %s", foreign.Body)
	}
	listed, _ := photos.ByUserID(context.Background(), sampleID)
	for _, photo := range listed {
		deleteReq := func(token string) *http.Request {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/photos/"+photo.ID, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			return req
		}
		request(deleteReq("foreign"), 404)
		request(deleteReq("valid"), 204)
		request(deleteReq("valid"), 404)
	}
	response = request(getProfile("valid"), 200)
	if !strings.Contains(response.Body.String(), `"name":"Анна"`) || !strings.Contains(response.Body.String(), `"photos":[]`) || len(storage.objects) != 0 {
		t.Fatalf("last photo removal must preserve profile and remove objects: %s", response.Body)
	}
	listed, _ = photos.ByUserID(context.Background(), sampleID)
	state := domain.SearchState{HasProfile: true, HasPreferences: true, PhotoCount: len(listed)}
	if !errors.Is(state.CheckEligible(), domain.ErrNoPhotos) {
		t.Fatal("a profile with no photos is not eligible for future discovery")
	}
}

func TestPhotoHTTPStorageFailuresAndDeleteRetry(t *testing.T) {
	photos := &photoFlowRepository{photos: make(map[string]domain.Photo)}
	storage := &photoFlowStorage{objects: make(map[string][]byte)}
	router := NewRouter(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Auth: photoFlowAuth{},
		Media: appmedia.NewService(photos, storage),
	})
	upload := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, photoRequest(t, photoPNG(t), 1, "valid"))
		if response.Code != want || strings.Contains(response.Body.String(), "SECRET") {
			t.Fatalf("expected %d and no storage details, got %d %s", want, response.Code, response.Body)
		}
	}
	storage.putErr = errors.New("SECRET S3 failure")
	upload(500)
	if len(photos.photos) != 0 || len(storage.objects) != 0 {
		t.Fatal("failed put must not create metadata")
	}
	storage.putErr = nil
	photos.createErr = errors.New("SECRET DB failure")
	upload(500)
	if len(photos.photos) != 0 || len(storage.objects) != 0 {
		t.Fatal("failed metadata write must compensate the stored object")
	}
	photos.createErr = domain.ErrNotFound
	upload(409)
	photos.createErr = nil
	upload(201)
	listed, _ := photos.ByUserID(context.Background(), sampleID)
	deletePhoto := func(want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/photos/"+listed[0].ID, nil)
		request.Header.Set("Authorization", "Bearer valid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || strings.Contains(response.Body.String(), "SECRET") {
			t.Fatalf("expected %d, got %d %s", want, response.Code, response.Body)
		}
	}
	storage.deleteErr = errors.New("SECRET delete failure")
	deletePhoto(500)
	if len(photos.photos) != 1 || len(storage.objects) != 1 {
		t.Fatal("S3 delete failure must keep metadata")
	}
	storage.deleteErr = nil
	photos.deleteErr = errors.New("SECRET DB delete failure")
	deletePhoto(500)
	if len(photos.photos) != 1 || len(storage.objects) != 0 {
		t.Fatal("partial delete must remain retryable")
	}
	photos.deleteErr = nil
	deletePhoto(204)
	if len(photos.photos) != 0 {
		t.Fatal("retry must remove remaining metadata")
	}
}
