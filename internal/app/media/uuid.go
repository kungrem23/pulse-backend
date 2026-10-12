package media

import (
	"strings"

	"github.com/google/uuid"
)

func newUUID() string {
	return uuid.NewString()
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == strings.ToLower(value)
}
