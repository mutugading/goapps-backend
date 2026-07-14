package mbparam

import "errors"

// Domain errors for MB parameter operations.
var (
	// ErrCodeRequired is returned when code is empty.
	ErrCodeRequired = errors.New("mbparam: code is required")
	// ErrInvalidType is returned when type is not SCALAR or PICKLIST.
	ErrInvalidType = errors.New("mbparam: type must be SCALAR or PICKLIST")
	// ErrCreatedByRequired is returned when created_by is empty.
	ErrCreatedByRequired = errors.New("mbparam: created_by is required")
	// ErrAlreadyExists is returned when a parameter code already exists.
	ErrAlreadyExists = errors.New("mbparam: code already exists")
	// ErrParamCodeRequired is returned when an option's param_code is empty.
	ErrParamCodeRequired = errors.New("mbparam: param_code is required")
	// ErrOptionCodeRequired is returned when an option's code is empty.
	ErrOptionCodeRequired = errors.New("mbparam: option code is required")
	// ErrNotFound is returned when a parameter row is not found.
	ErrNotFound = errors.New("mbparam: not found")
	// ErrParamNotFound is returned when a required recipe parameter code is missing from mst_mb_param.
	ErrParamNotFound = errors.New("mbparam: required parameter code not found")
)
