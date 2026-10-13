package media

import (
	"errors"
	"net/http"
	"testing"

	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

func TestPhotoResponsesMatchOpenAPI(t *testing.T) {
	spec := contracttest.Load(t)
	body, contentType := uploadBody(t, formPart{"file", "photo.png", []byte("validated by app")}, formPart{"position", "", []byte("1")})
	for _, err := range []error{nil, appmedia.ErrPhotoTooLarge, appmedia.ErrUnsupportedPhoto, appmedia.ErrInvalidPosition, appmedia.ErrPositionOccupied, appmedia.ErrProfileRequired, appmedia.ErrCleanupFailed, errors.New("internal")} {
		spec.CheckResponse(t, http.MethodPost, "/api/v1/photos", serve(t, &serviceStub{err: err}, http.MethodPost, "/api/v1/photos", body, contentType, "valid", false))
	}
	for _, token := range []string{"", "blocked"} {
		spec.CheckResponse(t, http.MethodPost, "/api/v1/photos", serve(t, &serviceStub{}, http.MethodPost, "/api/v1/photos", body, contentType, token, false))
	}
	spec.CheckResponse(t, http.MethodPost, "/api/v1/photos", serve(t, &serviceStub{}, http.MethodPost, "/api/v1/photos", nil, "application/json", "valid", false))
	for _, err := range []error{nil, appmedia.ErrPhotoNotFound, errors.New("internal")} {
		spec.CheckResponse(t, http.MethodDelete, "/api/v1/photos/{photo_id}", serve(t, &serviceStub{err: err}, http.MethodDelete, "/api/v1/photos/"+photoID, nil, "", "valid", false))
	}
	spec.CheckResponse(t, http.MethodDelete, "/api/v1/photos/{photo_id}", serve(t, &serviceStub{}, http.MethodDelete, "/api/v1/photos/invalid", nil, "", "valid", false))
}
