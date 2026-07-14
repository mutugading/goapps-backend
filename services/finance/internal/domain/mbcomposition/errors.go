package mbcomposition

import "errors"

// Domain errors for MB composition operations.
var (
	// ErrMbhIDRequired is returned when mbh_id is empty.
	ErrMbhIDRequired = errors.New("mbcomposition: mbh_id is required")
	// ErrInvalidSourceType is returned when source_type is not one of the known constants.
	ErrInvalidSourceType = errors.New("mbcomposition: source_type must be GROUP, MB, or CARRIER")
	// ErrGroupHeadIDRequired is returned when source_type is GROUP but group_head_id is empty.
	ErrGroupHeadIDRequired = errors.New("mbcomposition: group_head_id is required when source_type is GROUP")
	// ErrCreatedByRequired is returned when created_by is empty.
	ErrCreatedByRequired = errors.New("mbcomposition: created_by is required")
	// ErrNotFound is returned when a composition row is not found.
	ErrNotFound = errors.New("mbcomposition: not found")
)
