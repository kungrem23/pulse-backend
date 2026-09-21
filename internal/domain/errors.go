package domain

import "errors"

// ErrNotFound is returned by repositories when an aggregate does not exist.
// Repository implementations should wrap this error with operation context.
var ErrNotFound = errors.New("not found")
