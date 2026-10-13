package media

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	appmedia "github.com/kungrem23/pulse-backend/internal/app/media"
	"github.com/kungrem23/pulse-backend/internal/domain"
)

// MaxUploadBodyBytes includes up to 64 KiB for multipart headers and boundaries.
// Files themselves remain limited to exactly 10 MiB. Parsing uses no temp files.
const MaxUploadBodyBytes = domain.MaxPhotoBytes + (64 << 10)

var (
	errInvalidMultipart       = errors.New("invalid multipart upload")
	errUnsupportedContentType = errors.New("unsupported upload content type")
)

func readUpload(w http.ResponseWriter, r *http.Request) (appmedia.UploadCommand, error) {
	var command appmedia.UploadCommand
	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "multipart/form-data" {
		return command, errUnsupportedContentType
	}
	if params["boundary"] == "" {
		return command, errInvalidMultipart
	}
	if r.ContentLength > MaxUploadBodyBytes {
		return command, appmedia.ErrPhotoTooLarge
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBodyBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		return command, errInvalidMultipart
	}
	var hasFile, hasPosition bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return command, multipartError(err)
		}
		switch part.FormName() {
		case "file":
			if hasFile || part.FileName() == "" {
				return command, errInvalidMultipart
			}
			hasFile = true
			command.Content, err = io.ReadAll(io.LimitReader(part, domain.MaxPhotoBytes+1))
			if err != nil {
				return command, multipartError(err)
			}
			if len(command.Content) > domain.MaxPhotoBytes {
				return command, appmedia.ErrPhotoTooLarge
			}
		case "position":
			if hasPosition || part.FileName() != "" {
				return command, errInvalidMultipart
			}
			hasPosition = true
			value, err := io.ReadAll(io.LimitReader(part, 65))
			if err != nil {
				return command, multipartError(err)
			}
			if len(value) > 64 {
				return command, appmedia.ErrInvalidPosition
			}
			position, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 16)
			if err != nil || position < 1 || position > domain.MaxPhotos {
				return command, appmedia.ErrInvalidPosition
			}
			command.Position = int16(position)
		default:
			// Reject user_id, duplicate or unknown fields before invoking a service.
			return command, errInvalidMultipart
		}
		if err := part.Close(); err != nil {
			return command, multipartError(err)
		}
	}
	if !hasFile || !hasPosition {
		return command, errInvalidMultipart
	}
	// Consume epilogue too: chunked/unknown-length bodies cannot evade the total
	// body limit by appending bytes after the closing multipart boundary.
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return command, multipartError(err)
	}
	return command, nil
}

func multipartError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return tooLarge
	}
	return errInvalidMultipart
}
