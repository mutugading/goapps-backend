package master

import "errors"

// Sentinel errors returned by Repository implementations; the application
// layer maps them to apperr codes.
var (
	ErrNotFound        = errors.New("master: not found")
	ErrVersionMismatch = errors.New("master: version mismatch")
	ErrDuplicate       = errors.New("master: duplicate")
	ErrReference       = errors.New("master: invalid reference")
)
