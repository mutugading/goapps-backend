package grpc

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

const errorDomain = "payroll"

// grpcCode maps a payroll error code to a gRPC code (HTTP follows via
// grpcCodeToHTTPStatus; the exact 07 status stays in apperr.Code.HTTPStatus).
func grpcCode(c apperr.Code) codes.Code {
	switch c {
	case apperr.CodeValidation:
		return codes.InvalidArgument
	case apperr.CodeUnauthenticated:
		return codes.Unauthenticated
	case apperr.CodePermissionDenied, apperr.CodeDivisionForbidden:
		return codes.PermissionDenied
	case apperr.CodeNotFound:
		return codes.NotFound
	case apperr.CodeConflict, apperr.CodePeriodClosed, apperr.CodePeriodApproved, apperr.CodeRunInProgress:
		return codes.AlreadyExists
	case apperr.CodeVersionMismatch:
		return codes.FailedPrecondition
	case apperr.CodeMissingAttendance, apperr.CodeMissingAttdDays, apperr.CodeRuleIncomplete, apperr.CodeBusinessRule:
		return codes.FailedPrecondition
	default:
		return codes.Internal
	}
}

// ToStatusError converts err into a gRPC status error carrying an ErrorInfo
// (reason = payroll code). Non-payroll errors pass through unchanged, except
// that they must not leak internals: callers log them and return Internal.
func ToStatusError(err error) error {
	if err == nil {
		return nil
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		return err
	}
	st := status.New(grpcCode(ae.Code), ae.Message)
	if withInfo, derr := st.WithDetails(&errdetails.ErrorInfo{
		Reason: string(ae.Code), Domain: errorDomain, Metadata: ae.Details,
	}); derr == nil {
		st = withInfo
	}
	return st.Err()
}

// CodeFromError extracts the payroll code from a status error built by
// ToStatusError; ok is false when none is attached.
func CodeFromError(err error) (apperr.Code, bool) {
	for _, d := range status.Convert(err).Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok && info.GetDomain() == errorDomain {
			return apperr.Code(info.GetReason()), true
		}
	}
	return "", false
}
