package grpc

import (
	"context"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

func scopedCtx(t *testing.T, pairs ...any) context.Context {
	t.Helper()
	var scopes []scope.Scope
	for i := 0; i < len(pairs); i += 2 {
		sc, err := scope.New(int16(pairs[i].(int)), pairs[i+1].(string))
		if err != nil {
			t.Fatal(err)
		}
		scopes = append(scopes, sc)
	}
	return scope.WithIdentity(context.Background(), scope.Identity{UserID: "u1", Scopes: scopes})
}

func TestResolveCreateCompany(t *testing.T) {
	dry, commit := payrollv1.RunMode_RUN_MODE_DRY, payrollv1.RunMode_RUN_MODE_COMMIT
	tests := []struct {
		name     string
		ctx      context.Context
		company  int32
		mode     payrollv1.RunMode
		want     int16
		wantCode apperr.Code
	}{
		{"dry 0 one company", scopedCtx(t, 1, "SOLO", 1, "JAKARTA"), 0, dry, 1, ""},
		{"dry 0 picks the company of the division", scopedCtx(t, 2, "SOLO", 1, "JAKARTA"), 0, dry, 2, ""},
		{"dry 0 two companies is ambiguous", scopedCtx(t, 1, "SOLO", 2, "SOLO"), 0, dry, 0, apperr.CodeValidation},
		{"dry 0 without scope for the division", scopedCtx(t, 1, "JAKARTA"), 0, dry, 0, apperr.CodeDivisionForbidden},
		{"dry 0 without any scope", scopedCtx(t), 0, dry, 0, apperr.CodeDivisionForbidden},
		{"commit 0 is refused", scopedCtx(t, 1, "SOLO"), 0, commit, 0, apperr.CodeValidation},
		{"unspecified mode 0 is refused", scopedCtx(t, 1, "SOLO"), 0, payrollv1.RunMode_RUN_MODE_UNSPECIFIED, 0, apperr.CodeValidation},
		{"dry explicit company in scope", scopedCtx(t, 1, "SOLO", 2, "SOLO"), 2, dry, 2, ""},
		{"dry explicit company out of scope", scopedCtx(t, 1, "SOLO"), 2, dry, 0, apperr.CodeDivisionForbidden},
		{"commit explicit company in scope", scopedCtx(t, 1, "SOLO"), 1, commit, 1, ""},
		{"negative company", scopedCtx(t, 1, "SOLO"), -1, dry, 0, apperr.CodeValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCreateCompany(tc.ctx, &payrollv1.CreateRunRequest{Division: "SOLO", CompanyId: tc.company, Mode: tc.mode})
			if tc.wantCode == "" {
				if err != nil || got != tc.want {
					t.Fatalf("got (%d, %v), want %d", got, err, tc.want)
				}
				return
			}
			ae, isApp := err.(*apperr.Error) //nolint:errorlint // returned unwrapped
			if !isApp || ae.Code != tc.wantCode {
				t.Fatalf("err = %v, want code %s", err, tc.wantCode)
			}
		})
	}
}
