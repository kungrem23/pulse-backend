package domain

import "errors"

// ErrNotFound is returned by repositories when an aggregate does not exist.
// Repository implementations should wrap this error with operation context.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write violates a domain uniqueness or state
// constraint, such as registering an email that already exists.
var ErrConflict = errors.New("conflict")
