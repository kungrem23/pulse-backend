package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	testUserID   = "11111111-1111-4111-8111-111111111111"
	testPhotoID  = "22222222-2222-4222-8222-222222222222"
	testPhotoURL = "https://media.example.test/photo?signature=test"
	webPFixture  = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
)

type photoRepositoryStub struct {
	created       domain.Photo
	createCalls   int
	createErr     error
	owned         domain.Photo
	ownedErr      error
	lookupPhotoID string
	lookupUserID  string
	deleteCalls   int
	deletePhotoID string
	deleteUserID  string
	deleteErr     error
}

func (s *photoRepositoryStub) Create(_ context.Context, photo domain.Photo) error {
	s.createCalls++
	s.created = photo
	return s.createErr
}

func (s *photoRepositoryStub) ByIDForUser(_ context.Context, photoID, userID string) (domain.Photo, error) {
	s.lookupPhotoID = photoID
	s.lookupUserID = userID
	return s.owned, s.ownedErr
}

func (s *photoRepositoryStub) ByUserID(context.Context, string) ([]domain.Photo, error) {
	return nil, errors.New("unexpected photo list call in upload/delete test")
}

func (s *photoRepositoryStub) Delete(_ context.Context, photoID, userID string) error {
	s.deleteCalls++
	s.deletePhotoID = photoID
	s.deleteUserID = userID
	return s.deleteErr
}

type photoStorageStub struct {
	putCalls         int
	putKey           string
	putContent       []byte
	putContentType   string
	putResult        string
	putErr           error
	URLCalls         int
	URLKey           string
	URLResult        string
	URLErr           error
	deleteCalls      int
	deletedObjectKey string
	deleteErr        error
	deleteCtxErr     error
}

func (s *photoStorageStub) Put(_ context.Context, key string, content []byte, contentType string) (string, error) {
	s.putCalls++
	s.putKey = key
	s.putContent = append([]byte(nil), content...)
	s.putContentType = contentType
	if s.putResult == "" {
		s.putResult = key
	}
	return s.putResult, s.putErr
}

func (s *photoStorageStub) Delete(ctx context.Context, key string) error {
	s.deleteCalls++
	s.deletedObjectKey = key
	s.deleteCtxErr = ctx.Err()
	return s.deleteErr
}

func (s *photoStorageStub) URL(_ context.Context, key string) (string, error) {
	s.URLCalls++
	s.URLKey = key
	if s.URLResult == "" {
		s.URLResult = testPhotoURL
	}
	return s.URLResult, s.URLErr
}

func TestUploadStoresSupportedImageFormats(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		content     []byte
		contentType string
		extension   string
	}{
		{name: "jpeg", content: encodeImage(t, "jpeg"), contentType: "image/jpeg", extension: ".jpg"},
		{name: "png", content: encodeImage(t, "png"), contentType: "image/png", extension: ".png"},
		{name: "webp", content: decodeWebP(t), contentType: "image/webp", extension: ".webp"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &photoRepositoryStub{}
			storage := &photoStorageStub{}
			service := newService(repository, storage, func() string { return testPhotoID })

			result, err := service.Upload(context.Background(), UploadCommand{
				UserID: testUserID, Content: testCase.content, Position: 3,
			})
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if result != (Photo{ID: testPhotoID, URL: testPhotoURL, Position: 3}) {
				t.Fatalf("unexpected result: %#v", result)
			}
			if storage.putContentType != testCase.contentType || !bytes.Equal(storage.putContent, testCase.content) {
				t.Fatal("storage did not receive validated image data")
			}
			if !strings.HasSuffix(storage.putKey, "/"+testPhotoID+testCase.extension) {
				t.Fatalf("unexpected object key: %s", storage.putKey)
			}
			if repository.created != (domain.Photo{
				ID: testPhotoID, UserID: testUserID, ObjectKey: storage.putKey, Position: 3,
			}) {
				t.Fatalf("unexpected metadata: %#v", repository.created)
			}
		})
	}
}

func TestUploadRejectsInvalidInputBeforeStorage(t *testing.T) {
	validImage := encodeImage(t, "png")
	for _, testCase := range []struct {
		name    string
		command UploadCommand
		wantErr error
	}{
		{name: "invalid user ID", command: UploadCommand{UserID: "not-a-uuid", Content: validImage, Position: 1}, wantErr: ErrInvalidUserID},
		{name: "position zero", command: UploadCommand{UserID: testUserID, Content: validImage, Position: 0}, wantErr: ErrInvalidPosition},
		{name: "position six", command: UploadCommand{UserID: testUserID, Content: validImage, Position: 6}, wantErr: ErrInvalidPosition},
		{name: "empty", command: UploadCommand{UserID: testUserID, Position: 1}, wantErr: ErrUnsupportedPhoto},
		{name: "fake png", command: UploadCommand{UserID: testUserID, Content: append([]byte("\x89PNG\r\n\x1a\n"), []byte("not an image")...), Position: 1}, wantErr: ErrUnsupportedPhoto},
		{name: "too large", command: UploadCommand{UserID: testUserID, Content: make([]byte, domain.MaxPhotoBytes+1), Position: 1}, wantErr: ErrPhotoTooLarge},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			storage := &photoStorageStub{}
			service := newService(&photoRepositoryStub{}, storage, func() string { return testPhotoID })
			_, err := service.Upload(context.Background(), testCase.command)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("expected %v, got %v", testCase.wantErr, err)
			}
			if storage.putCalls != 0 {
				t.Fatal("invalid input reached storage")
			}
		})
	}
}

func TestUploadCompensatesObjectWhenMetadataConflicts(t *testing.T) {
	repository := &photoRepositoryStub{createErr: errors.Join(errors.New("unique constraint"), domain.ErrConflict)}
	storage := &photoStorageStub{}
	service := newService(repository, storage, func() string { return testPhotoID })

	_, err := service.Upload(context.Background(), UploadCommand{
		UserID: testUserID, Content: encodeImage(t, "png"), Position: 1,
	})
	if !errors.Is(err, ErrPositionOccupied) {
		t.Fatalf("expected occupied position, got %v", err)
	}
	if storage.deleteCalls != 1 || storage.deletedObjectKey != storage.putKey {
		t.Fatal("stored object was not compensated")
	}
}

func TestUploadCompensatesObjectWhenURLFails(t *testing.T) {
	urlErr := errors.New("signing unavailable")
	repository := &photoRepositoryStub{}
	storage := &photoStorageStub{URLErr: urlErr}
	service := newService(repository, storage, func() string { return testPhotoID })

	_, err := service.Upload(context.Background(), UploadCommand{
		UserID: testUserID, Content: encodeImage(t, "jpeg"), Position: 1,
	})
	if !errors.Is(err, urlErr) {
		t.Fatalf("expected URL error, got %v", err)
	}
	if storage.deleteCalls != 1 || repository.createCalls != 0 {
		t.Fatal("URL failure was not compensated before metadata creation")
	}
}

func TestUploadReportsCompensationFailure(t *testing.T) {
	createErr := errors.New("database unavailable")
	cleanupErr := errors.New("storage unavailable")
	repository := &photoRepositoryStub{createErr: createErr}
	storage := &photoStorageStub{deleteErr: cleanupErr}
	service := newService(repository, storage, func() string { return testPhotoID })

	_, err := service.Upload(context.Background(), UploadCommand{
		UserID: testUserID, Content: encodeImage(t, "png"), Position: 1,
	})
	if !errors.Is(err, createErr) || !errors.Is(err, cleanupErr) || !errors.Is(err, ErrCleanupFailed) {
		t.Fatalf("expected both failures, got %v", err)
	}
}

func TestUploadRequiresProfileAndCompensatesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repository := &photoRepositoryStub{createErr: domain.ErrNotFound}
	storage := &photoStorageStub{}
	service := newService(repository, storage, func() string { return testPhotoID })
	_, err := service.Upload(ctx, UploadCommand{UserID: testUserID, Content: encodeImage(t, "png"), Position: 1})
	if !errors.Is(err, ErrProfileRequired) || storage.deleteCalls != 1 || storage.deleteCtxErr != nil {
		t.Fatalf("missing profile must trigger cleanup independent of request cancellation: %v, %+v", err, storage)
	}
}

func TestUploadRejectsTruncatedAndOversizedDimensions(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "webp"} {
		t.Run(format, func(t *testing.T) {
			var content []byte
			if format == "webp" {
				content = decodeWebP(t)
			} else {
				content = encodeImage(t, format)
			}
			content = content[:len(content)-10]
			if _, _, err := image.DecodeConfig(bytes.NewReader(content)); err != nil {
				t.Fatalf("fixture must have a valid header to exercise full decoding: %v", err)
			}
			storage := &photoStorageStub{}
			service := newService(&photoRepositoryStub{}, storage, func() string { return testPhotoID })
			_, err := service.Upload(context.Background(), UploadCommand{UserID: testUserID, Content: content, Position: 1})
			if !errors.Is(err, ErrUnsupportedPhoto) || storage.putCalls != 0 {
				t.Fatalf("truncated %s reached storage: %v", format, err)
			}
		})
	}
	content := encodeImage(t, "png")
	binary.BigEndian.PutUint32(content[16:20], MaxDecodedPhotoPixels+1)
	binary.BigEndian.PutUint32(content[20:24], 1)
	binary.BigEndian.PutUint32(content[29:33], crc32.ChecksumIEEE(content[12:29]))
	if _, _, err := image.DecodeConfig(bytes.NewReader(content)); err != nil {
		t.Fatalf("large-dimension fixture must have a valid PNG header: %v", err)
	}
	if _, err := inspectImage(content); !errors.Is(err, ErrUnsupportedPhoto) {
		t.Fatalf("huge decoded dimensions must be rejected before allocation: %v", err)
	}
}

func TestUploadAcceptsExactFileSizeLimit(t *testing.T) {
	content := encodeImage(t, "png")
	content = append(content, make([]byte, domain.MaxPhotoBytes-len(content))...)
	service := newService(&photoRepositoryStub{}, &photoStorageStub{}, func() string { return testPhotoID })
	if _, err := service.Upload(context.Background(), UploadCommand{UserID: testUserID, Content: content, Position: 5}); err != nil {
		t.Fatalf("valid image at exactly 10 MiB must be accepted: %v", err)
	}
}

func TestDeleteRemovesObjectThenOwnedMetadata(t *testing.T) {
	objectKey := "users/owner/photos/photo.png"
	repository := &photoRepositoryStub{owned: domain.Photo{
		ID: testPhotoID, UserID: testUserID, ObjectKey: objectKey, Position: 1,
	}}
	storage := &photoStorageStub{}
	service := newService(repository, storage, func() string { return testPhotoID })

	if err := service.Delete(context.Background(), testUserID, testPhotoID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if repository.lookupPhotoID != testPhotoID || repository.lookupUserID != testUserID ||
		repository.deletePhotoID != testPhotoID || repository.deleteUserID != testUserID {
		t.Fatal("repository operations were not scoped to the owner")
	}
	if storage.deletedObjectKey != objectKey || repository.deleteCalls != 1 {
		t.Fatal("photo was not deleted from both stores")
	}
}

func TestDeleteRejectsMissingOrForeignPhoto(t *testing.T) {
	repository := &photoRepositoryStub{ownedErr: domain.ErrNotFound}
	storage := &photoStorageStub{}
	service := newService(repository, storage, func() string { return testPhotoID })

	err := service.Delete(context.Background(), testUserID, testPhotoID)
	if !errors.Is(err, ErrPhotoNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if storage.deleteCalls != 0 || repository.deleteCalls != 0 {
		t.Fatal("missing or foreign photo reached deletion")
	}
}

func TestDeleteKeepsMetadataWhenObjectDeletionFails(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	repository := &photoRepositoryStub{owned: domain.Photo{ObjectKey: "photo.png"}}
	storage := &photoStorageStub{deleteErr: storageErr}
	service := newService(repository, storage, func() string { return testPhotoID })

	err := service.Delete(context.Background(), testUserID, testPhotoID)
	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error, got %v", err)
	}
	if repository.deleteCalls != 0 {
		t.Fatal("metadata was deleted after storage failure")
	}
}

func TestDeleteCanRetryAfterMetadataFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	repository := &photoRepositoryStub{
		owned:     domain.Photo{ObjectKey: "photo.png"},
		deleteErr: databaseErr,
	}
	storage := &photoStorageStub{}
	service := newService(repository, storage, func() string { return testPhotoID })

	err := service.Delete(context.Background(), testUserID, testPhotoID)
	if !errors.Is(err, databaseErr) {
		t.Fatalf("expected database error, got %v", err)
	}
	if storage.deleteCalls != 1 || repository.deleteCalls != 1 {
		t.Fatal("expected storage delete followed by metadata delete")
	}
	repository.deleteErr = nil
	if err := service.Delete(context.Background(), testUserID, testPhotoID); err != nil || storage.deleteCalls != 2 || repository.deleteCalls != 2 {
		t.Fatalf("retry must repair a metadata deletion failure: %v", err)
	}
}

func TestDeleteRejectsInvalidIDs(t *testing.T) {
	service := newService(&photoRepositoryStub{}, &photoStorageStub{}, func() string { return testPhotoID })
	if err := service.Delete(context.Background(), "invalid", testPhotoID); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("expected invalid user ID, got %v", err)
	}
	if err := service.Delete(context.Background(), testUserID, "invalid"); !errors.Is(err, ErrInvalidPhotoID) {
		t.Fatalf("expected invalid photo ID, got %v", err)
	}
}

func encodeImage(t *testing.T, format string) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 2, 2))
	value.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buffer, value, nil)
	case "png":
		err = png.Encode(&buffer, value)
	default:
		t.Fatalf("unsupported fixture format %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s fixture: %v", format, err)
	}
	return buffer.Bytes()
}

func decodeWebP(t *testing.T) []byte {
	t.Helper()
	content, err := base64.StdEncoding.DecodeString(webPFixture)
	if err != nil {
		t.Fatalf("decode WebP fixture: %v", err)
	}
	return content
}
