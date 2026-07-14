package mbworkflowlog

import "errors"

// Domain errors for MB workflow-log operations.
var (
	// ErrMbhIDRequired is returned when mbh_id is empty.
	ErrMbhIDRequired = errors.New("mbworkflowlog: mbh_id is required")
	// ErrToStateRequired is returned when to_state is empty.
	ErrToStateRequired = errors.New("mbworkflowlog: to_state is required")
)
