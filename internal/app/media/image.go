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
	if width <= 0 || height <= 0 {
		return imageFormat{}, ErrUnsupportedPhoto
	}

	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}
	return imageFormat{contentType: detectedType, extension: extensions[detectedType]}, nil
}
