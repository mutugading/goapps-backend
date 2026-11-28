package master

import "errors"

// Sentinel errors returned by Repository implementations; the application
// layer maps them to apperr codes.
var (
	ErrNotFound        = errors.New("master: not found")
	ErrVersionMismatch = errors.New("master: version mismatch")
	ErrDuplicate       = errors.New("master: duplicate")
	ErrReference       = errors.New("master: invalid reference")
	// ErrTaxYearFinal: the tax year of a backdated tax-status correction already has an APPROVED/CLOSED
	// December (or leaver) period (T033n2).
	ErrTaxYearFinal = errors.New("master: tax year already final")
)
