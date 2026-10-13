package domain

import "errors"

// ErrNotFound is returned by repositories when an aggregate does not exist.
// Repository implementations should wrap this error with operation context.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write violates a domain uniqueness or state
// constraint, such as registering an email that already exists.
var ErrConflict = errors.New("conflict")

// Shared input and discovery errors are compared with errors.Is by app/HTTP.
// Eligibility errors (incomplete profile, no photos, sleep, block) live in
// eligibility.go and map to 403 on feed/swipe.
var (
	ErrInvalidUserID        = errors.New("invalid user ID")
	ErrInvalidCursor        = errors.New("invalid pagination cursor")
	ErrInvalidLimit         = errors.New("page limit must be between 1 and 100")
	ErrSelfSwipe            = errors.New("cannot swipe yourself")
	ErrCandidateUnavailable = errors.New("candidate is unavailable")
)
