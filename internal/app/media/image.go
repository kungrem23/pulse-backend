package media

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	"github.com/kungrem23/pulse-backend/internal/domain"
	"golang.org/x/image/webp"
)

type imageFormat struct {
	contentType string
	extension   string
}

// MaxDecodedPhotoPixels bounds memory used by full image validation. A small
// compressed file can otherwise request gigabytes of image buffers.
const MaxDecodedPhotoPixels = 25_000_000

func inspectImage(content []byte) (imageFormat, error) {
	if len(content) > domain.MaxPhotoBytes {
		return imageFormat{}, ErrPhotoTooLarge
	}
	if len(content) == 0 {
		return imageFormat{}, ErrUnsupportedPhoto
	}

	detectedType := http.DetectContentType(content)
	var (
		width  int
		height int
	)
	switch detectedType {
	case "image/jpeg":
		config, format, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil || format != "jpeg" {
			return imageFormat{}, ErrUnsupportedPhoto
		}
		width, height = config.Width, config.Height
	case "image/png":
		config, format, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil || format != "png" {
			return imageFormat{}, ErrUnsupportedPhoto
		}
		width, height = config.Width, config.Height
	case "image/webp":
		config, err := webp.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			return imageFormat{}, ErrUnsupportedPhoto
		}
		width, height = config.Width, config.Height
	default:
		return imageFormat{}, ErrUnsupportedPhoto
	}
	if width <= 0 || height <= 0 || width > MaxDecodedPhotoPixels/height {
		return imageFormat{}, ErrUnsupportedPhoto
	}

	// DecodeConfig alone accepts truncated images with valid headers. Validate
	// the complete image only after bounding the decompressed dimensions.
	decoded, format, err := image.Decode(bytes.NewReader(content))
	if err != nil || decoded == nil {
		return imageFormat{}, ErrUnsupportedPhoto
	}
	wantFormat := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}
	if format != wantFormat[detectedType] {
		return imageFormat{}, ErrUnsupportedPhoto
	}

	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}
	return imageFormat{contentType: detectedType, extension: extensions[detectedType]}, nil
}
