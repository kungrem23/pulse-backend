package media

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/transport/http/middleware"
)

const (
	ownerID = "11111111-1111-4111-8111-111111111111"
	photoID = "22222222-2222-4222-8222-222222222222"
)

type serviceStub struct {
	command     appmedia.UploadCommand
	uploadCalls int
	deleteCalls int
	deleteOwner string
	deletePhoto string
	err         error
}

func (s *serviceStub) Upload(_ context.Context, command appmedia.UploadCommand) (appmedia.Photo, error) {
	s.uploadCalls++
	s.command = command
	return appmedia.Photo{ID: photoID, URL: "https://photos.example.test/signed", Position: command.Position}, s.err
}

func (s *serviceStub) Delete(_ context.Context, owner, id string) error {
	s.deleteCalls++
	s.deleteOwner, s.deletePhoto = owner, id
	return s.err
}

type authenticatorStub struct{}

func (authenticatorStub) Authenticate(_ context.Context, token string) (string, error) {
	if token == "blocked" {
		return "", appauth.ErrUserBlocked
	}
	if token != "valid" {
		return "", appauth.ErrInvalidAccessToken
	}
	return ownerID, nil
}

type formPart struct {
	name     string
	filename string
	content  []byte
}

func uploadBody(t *testing.T, parts ...formPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.filename == "" {
			if err := writer.WriteField(part.name, string(part.content)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		file, err := writer.CreateFormFile(part.name, part.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(part.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func serve(t *testing.T, service appmedia.Service, method, path string, body []byte, contentType, token string, unknownLength bool) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service), middleware.RequireUser(authenticatorStub{}))
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if unknownLength {
		request.ContentLength = -1
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestUploadUsesVerifiedOwnerAndReturnsPhoto(t *testing.T) {
	content := []byte("content validated by app service")
	for _, positionFirst := range []bool{false, true} {
		parts := []formPart{{"file", "arbitrary.extension", content}, {"position", "", []byte("2")}}
		if positionFirst {
			parts[0], parts[1] = parts[1], parts[0]
		}
		body, contentType := uploadBody(t, parts...)
		service := &serviceStub{}
		response := serve(t, service, http.MethodPost, "/api/v1/photos?user_id=foreign", body, contentType, "valid", false)
		if response.Code != http.StatusCreated || service.command.UserID != ownerID ||
			service.command.Position != 2 || !bytes.Equal(service.command.Content, content) ||
			strings.TrimSpace(response.Body.String()) != `{"id":"`+photoID+`","url":"https://photos.example.test/signed","position":2}` {
			t.Fatalf("unexpected upload: %d %s, %+v", response.Code, response.Body, service.command)
		}
	}
}

func TestUploadRejectsInvalidMultipartBeforeService(t *testing.T) {
	file := formPart{"file", "photo.png", []byte("file bytes")}
	position := formPart{"position", "", []byte("1")}
	for _, test := range []struct {
		name   string
		parts  []formPart
		status int
	}{
		{"no file", []formPart{position}, 400},
		{"no position", []formPart{file}, 400},
		{"duplicate file", []formPart{file, file, position}, 400},
		{"duplicate position", []formPart{file, position, position}, 400},
		{"foreign owner", []formPart{file, position, {"user_id", "", []byte("foreign")}}, 400},
		{"unknown field", []formPart{file, position, {"extra", "", []byte("value")}}, 400},
		{"file without filename", []formPart{{"file", "", []byte("file bytes")}, position}, 400},
		{"position is a file", []formPart{file, {"position", "p.txt", []byte("1")}}, 400},
		{"position zero", []formPart{file, {"position", "", []byte("0")}}, 422},
		{"position six", []formPart{file, {"position", "", []byte("6")}}, 422},
		{"position overflow", []formPart{file, {"position", "", []byte("65537")}}, 422},
		{"fractional position", []formPart{file, {"position", "", []byte("1.5")}}, 422},
		{"missing value", []formPart{file, {"position", "", nil}}, 422},
		{"long value", []formPart{file, {"position", "", bytes.Repeat([]byte("1"), 65)}}, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, contentType := uploadBody(t, test.parts...)
			service := &serviceStub{}
			response := serve(t, service, http.MethodPost, "/api/v1/photos", body, contentType, "valid", false)
			if response.Code != test.status || service.uploadCalls != 0 {
				t.Fatalf("expected %d and no service call, got %d: %s", test.status, response.Code, response.Body)
			}
		})
	}
	for _, test := range []struct {
		body, contentType string
		status            int
	}{
		{`{}`, "application/json", 415},
		{"", "", 415},
		{"", "multipart/form-data", 400},
		{"not a multipart body", "multipart/form-data; boundary=abc", 400},
	} {
		service := &serviceStub{}
		response := serve(t, service, http.MethodPost, "/api/v1/photos", []byte(test.body), test.contentType, "valid", false)
		if response.Code != test.status || service.uploadCalls != 0 {
			t.Fatalf("expected %d, got %d: %s", test.status, response.Code, response.Body)
		}
	}
}

func TestUploadBoundsFileAndWholeBody(t *testing.T) {
	for _, unknownLength := range []bool{false, true} {
		for _, size := range []int{domain.MaxPhotoBytes, domain.MaxPhotoBytes + 1} {
			body, contentType := uploadBody(t, formPart{"file", "photo.png", make([]byte, size)}, formPart{"position", "", []byte("1")})
			service := &serviceStub{}
			response := serve(t, service, http.MethodPost, "/api/v1/photos", body, contentType, "valid", unknownLength)
			want := http.StatusCreated
			if size > domain.MaxPhotoBytes {
				want = http.StatusRequestEntityTooLarge
			}
			if response.Code != want || (want == 413 && service.uploadCalls != 0) {
				t.Fatalf("size %d unknownLength=%v: expected %d, got %d %s", size, unknownLength, want, response.Code, response.Body)
			}
		}
		body, contentType := uploadBody(t, formPart{"file", "photo.png", []byte("tiny file")}, formPart{"position", "", []byte("1")})
		body = append(body, make([]byte, MaxUploadBodyBytes)...)
		service := &serviceStub{}
		response := serve(t, service, http.MethodPost, "/api/v1/photos", body, contentType, "valid", unknownLength)
		if response.Code != 413 || service.uploadCalls != 0 {
			t.Fatalf("multipart epilogue bypassed body limit: %d %s", response.Code, response.Body)
		}
	}
}

func TestMediaErrorMappingsAndOwnership(t *testing.T) {
	body, contentType := uploadBody(t, formPart{"file", "photo.png", []byte("data")}, formPart{"position", "", []byte("1")})
	for _, test := range []struct {
		err  error
		want int
	}{
		{appmedia.ErrPhotoTooLarge, 413}, {appmedia.ErrUnsupportedPhoto, 415},
		{appmedia.ErrInvalidPosition, 422}, {appmedia.ErrPositionOccupied, 409},
		{appmedia.ErrProfileRequired, 409}, {appmedia.ErrInvalidUserID, 401},
		{errors.Join(appmedia.ErrPositionOccupied, appmedia.ErrCleanupFailed), 500},
		{errors.New("S3 secret and private object_key"), 500},
	} {
		response := serve(t, &serviceStub{err: test.err}, http.MethodPost, "/api/v1/photos", body, contentType, "valid", false)
		if response.Code != test.want || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "object_key") {
			t.Fatalf("error %v: expected %d, got %d %s", test.err, test.want, response.Code, response.Body)
		}
	}
	for _, test := range []struct {
		id   string
		err  error
		want int
	}{
		{photoID, nil, 204}, {photoID, appmedia.ErrPhotoNotFound, 404},
		{photoID, errors.New("private object_key"), 500}, {"not-a-uuid", nil, 400},
	} {
		service := &serviceStub{err: test.err}
		response := serve(t, service, http.MethodDelete, "/api/v1/photos/"+test.id, nil, "", "valid", false)
		if response.Code != test.want || strings.Contains(response.Body.String(), "object_key") {
			t.Fatalf("delete: expected %d, got %d %s", test.want, response.Code, response.Body)
		}
		if test.want != 400 && (service.deleteOwner != ownerID || service.deletePhoto != photoID) {
			t.Fatalf("delete must use the JWT owner: %+v", service)
		}
		if test.want == 400 && service.deleteCalls != 0 {
			t.Fatal("invalid UUID reached service")
		}
		if test.want == 204 && response.Body.Len() != 0 {
			t.Fatal("204 must have no body")
		}
	}
	for _, token := range []string{"", "refresh", "blocked"} {
		service := &serviceStub{}
		response := serve(t, service, http.MethodPost, "/api/v1/photos", body, contentType, token, false)
		want := 401
		if token == "blocked" {
			want = 403
		}
		if response.Code != want || service.uploadCalls != 0 {
			t.Fatalf("token %q: expected %d before service, got %d", token, want, response.Code)
		}
	}
	if response := serve(t, nil, http.MethodPost, "/api/v1/photos", body, contentType, "valid", false); response.Code != 500 {
		t.Fatalf("nil service: expected 500, got %d", response.Code)
	}
}
