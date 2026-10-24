// Package apperr defines the payroll business error codes of 07 (Konvensi umum, Error).
package apperr

import (
	"errors"
	"fmt"
)

// Code is a stable, client-visible error code.
type Code string

// Error codes (07 Konvensi umum, §4 Error khusus).
const (
	CodeValidation        Code = "VALIDATION_ERROR"
	CodeUnauthenticated   Code = "UNAUTHENTICATED"
	CodePermissionDenied  Code = "PERMISSION_DENIED"
	CodeDivisionForbidden Code = "DIVISION_FORBIDDEN"
	CodeNotFound          Code = "NOT_FOUND"
	CodeConflict          Code = "CONFLICT"
	CodeVersionMismatch   Code = "VERSION_MISMATCH"
	CodePeriodClosed      Code = "PERIOD_CLOSED"
	CodePeriodApproved    Code = "PERIOD_APPROVED"
	CodeRunInProgress     Code = "RUN_IN_PROGRESS"
	CodeMissingAttendance Code = "MISSING_ATTENDANCE"
	CodeMissingAttdDays   Code = "MISSING_ATTD_DAYS"
	CodeRuleIncomplete    Code = "RULE_INCOMPLETE"
	CodeBusinessRule      Code = "BUSINESS_RULE"
	CodeIdemConflict      Code = "IDEMPOTENCY_KEY_CONFLICT"
	CodeIdemInProgress    Code = "IDEMPOTENCY_IN_PROGRESS"
	CodeInternal          Code = "INTERNAL"
)

// Error is a coded business error carrying optional details.
type Error struct {
	Code    Code
	Message string
	Details map[string]string
}

// Error implements error.
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Is matches on Code so errors.Is(err, apperr.PeriodClosed(...)) works.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Code == e.Code
}

// New builds a coded error.
func New(code Code, msg string, details map[string]string) *Error {
	return &Error{Code: code, Message: msg, Details: details}
}

// PeriodClosed is returned when mutating a CLOSED period.
func PeriodClosed(yymm int, division string) *Error {
	return New(CodePeriodClosed, "periode sudah CLOSED",
		map[string]string{"yymm": fmt.Sprint(yymm), "division": division})
}

// PeriodApproved is returned when mutating an APPROVED period.
func PeriodApproved(yymm int, division string) *Error {
	return New(CodePeriodApproved, "periode sudah APPROVED",
		map[string]string{"yymm": fmt.Sprint(yymm), "division": division})
}

// DivisionForbidden is returned when the caller has no scope for company:division.
func DivisionForbidden(companyID int16, division string) *Error {
	return New(CodeDivisionForbidden, "divisi di luar scope pengguna",
		map[string]string{"company_id": fmt.Sprint(companyID), "division": division})
}

// HTTPStatus maps a code to the HTTP status of 07 (400, 403, 404, 409, 422).
func (c Code) HTTPStatus() int {
	switch c {
	case CodeValidation:
		return 400
	case CodeUnauthenticated:
		return 401
	case CodePermissionDenied, CodeDivisionForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeConflict, CodePeriodClosed, CodePeriodApproved, CodeRunInProgress, CodeIdemConflict, CodeIdemInProgress:
		return 409
	case CodeVersionMismatch:
		return 412
	case CodeMissingAttendance, CodeMissingAttdDays, CodeRuleIncomplete, CodeBusinessRule:
		return 422
	default:
		return 500
	}
}
