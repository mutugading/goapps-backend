// Package response provides utilities for building API responses.
package response

// BaseResponse fields.
const (
	StatusSuccess        = "200"
	StatusCreated        = "201"
	StatusBadRequest     = "400"
	StatusUnauthorized   = "401"
	StatusForbidden      = "403"
	StatusNotFound       = "404"
	StatusConflict       = "409"
	StatusInternalError  = "500"
	StatusServiceUnavail = "503"
)

// ValidationError represents a field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// BaseResponse is the standard response format.
type BaseResponse struct {
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	StatusCode       string            `json:"status_code"`
	IsSuccess        bool              `json:"is_success"`
	Message          string            `json:"message"`
}

// Success creates a successful response.
func Success(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusSuccess,
		IsSuccess:  true,
		Message:    message,
	}
}

// Created creates a successful creation response.
func Created(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusCreated,
		IsSuccess:  true,
		Message:    message,
	}
}

// BadRequest creates a bad request response.
func BadRequest(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusBadRequest,
		IsSuccess:  false,
		Message:    message,
	}
}

// NotFound creates a not found response.
func NotFound(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusNotFound,
		IsSuccess:  false,
		Message:    message,
	}
}

// Conflict creates a conflict response.
func Conflict(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusConflict,
		IsSuccess:  false,
		Message:    message,
	}
}

// InternalError creates an internal server error response.
func InternalError(message string) BaseResponse {
	return BaseResponse{
		StatusCode: StatusInternalError,
		IsSuccess:  false,
		Message:    message,
	}
}

// ValidationFailed creates a validation error response.
func ValidationFailed(errors []ValidationError) BaseResponse {
	return BaseResponse{
		ValidationErrors: errors,
		StatusCode:       StatusBadRequest,
		IsSuccess:        false,
		Message:          "Validation failed",
	}
}
