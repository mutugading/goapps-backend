package grpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

func TestToStatusError(t *testing.T) {
	tests := []struct {
		err      *apperr.Error
		wantGRPC codes.Code
		wantHTTP int
	}{
		{apperr.New(apperr.CodeValidation, "x", nil), codes.InvalidArgument, 400},
		{apperr.DivisionForbidden(1, "JAKARTA"), codes.PermissionDenied, 403},
		{apperr.New(apperr.CodeNotFound, "x", nil), codes.NotFound, 404},
		{apperr.PeriodClosed(202609, "SOLO"), codes.AlreadyExists, 409},
		{apperr.PeriodApproved(202609, "SOLO"), codes.AlreadyExists, 409},
		{apperr.New(apperr.CodeRunInProgress, "x", nil), codes.AlreadyExists, 409},
		{apperr.New(apperr.CodeVersionMismatch, "x", nil), codes.FailedPrecondition, 412},
		{apperr.New(apperr.CodeMissingAttendance, "x", nil), codes.FailedPrecondition, 422},
		{apperr.New(apperr.CodeRuleIncomplete, "x", nil), codes.FailedPrecondition, 422},
		{apperr.New(apperr.CodeInternal, "x", nil), codes.Internal, 500},
	}
	for _, tt := range tests {
		t.Run(string(tt.err.Code), func(t *testing.T) {
			// wrapped, as use cases will do
			got := ToStatusError(fmt.Errorf("ctx: %w", tt.err))
			if status.Code(got) != tt.wantGRPC {
				t.Fatalf("grpc code = %v, want %v", status.Code(got), tt.wantGRPC)
			}
			code, ok := CodeFromError(got)
			if !ok || code != tt.err.Code {
				t.Fatalf("reason = %q (%v), want %q", code, ok, tt.err.Code)
			}
			if h := tt.err.Code.HTTPStatus(); h != tt.wantHTTP {
				t.Fatalf("http = %d, want %d", h, tt.wantHTTP)
			}
		})
	}
}

func TestToStatusError_PassThrough(t *testing.T) {
	plain := errors.New("boom")
	if got := ToStatusError(plain); got != plain {
		t.Fatalf("plain error must pass through, got %v", got)
	}
	if ToStatusError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}

func TestErrorsIsByCode(t *testing.T) {
	if !errors.Is(apperr.PeriodClosed(1, "A"), apperr.PeriodClosed(2, "B")) {
		t.Fatal("errors.Is must match on code")
	}
}
