package mblusture

import "errors"

// Domain errors for MB lusture operations.
var (
	// ErrCodeRequired is returned when code is empty.
	ErrCodeRequired = errors.New("mblusture: code is required")
	// ErrCreatedByRequired is returned when created_by is empty.
	ErrCreatedByRequired = errors.New("mblusture: created_by is required")
	// ErrAlreadyExists is returned when a lusture code already exists.
	ErrAlreadyExists = errors.New("mblusture: code already exists")
	// ErrNotFound is returned when a lusture row is not found.
	ErrNotFound = errors.New("mblusture: not found")
)
