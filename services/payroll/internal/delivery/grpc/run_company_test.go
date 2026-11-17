package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
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
		wantMsg  string // substring of the message; the field detail must be company_id when set
	}{
		{"dry 0 one company", scopedCtx(t, 1, "SOLO", 1, "JAKARTA"), 0, dry, 1, "", ""},
		{"dry 0 picks the company of the division", scopedCtx(t, 2, "SOLO", 1, "JAKARTA"), 0, dry, 2, "", ""},
		{"dry 0 two companies is ambiguous", scopedCtx(t, 1, "SOLO", 2, "SOLO"), 0, dry, 0, apperr.CodeValidation, "ambigu"},
		{"dry 0 without scope for the division", scopedCtx(t, 1, "JAKARTA"), 0, dry, 0, apperr.CodeDivisionForbidden, ""},
		{"dry 0 without any scope", scopedCtx(t), 0, dry, 0, apperr.CodeDivisionForbidden, ""},
		{"commit 0 is refused", scopedCtx(t, 1, "SOLO"), 0, commit, 0, apperr.CodeValidation, "wajib"},
		{"unspecified mode 0 is refused", scopedCtx(t, 1, "SOLO"), 0, payrollv1.RunMode_RUN_MODE_UNSPECIFIED, 0, apperr.CodeValidation, ""},
		{"dry explicit company in scope", scopedCtx(t, 1, "SOLO", 2, "SOLO"), 2, dry, 2, "", ""},
		{"dry explicit company out of scope", scopedCtx(t, 1, "SOLO"), 2, dry, 0, apperr.CodeDivisionForbidden, ""},
		{"commit explicit company in scope", scopedCtx(t, 1, "SOLO"), 1, commit, 1, "", ""},
		{"negative company", scopedCtx(t, 1, "SOLO"), -1, dry, 0, apperr.CodeValidation, "tidak valid"},
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
			if tc.wantMsg == "" {
				return
			}
			if !strings.Contains(ae.Message, tc.wantMsg) || ae.Details[keyField] != "company_id" {
				t.Fatalf("message %q details %v, want %q and field company_id", ae.Message, ae.Details, tc.wantMsg)
			}
		})
	}
}

// Dry with company_id 0 and no identity at all (no JWT scope) is refused before any company is guessed.
func TestResolveCreateCompanyNoIdentity(t *testing.T) {
	got, err := resolveCreateCompany(context.Background(), &payrollv1.CreateRunRequest{
		Division: "SOLO", Mode: payrollv1.RunMode_RUN_MODE_DRY})
	if !errors.Is(err, scope.ErrNoIdentity) || got != 0 {
		t.Fatalf("got (%d, %v), want (0, ErrNoIdentity)", got, err)
	}
}

// company_id of the response comes from the resolved company, not from the request (dry with 0).
func TestRunPBCompany(t *testing.T) {
	if got := runPB(run.Info{CompanyID: 3}, false).GetCompanyId(); got != 3 {
		t.Fatalf("runPB company_id = %d, want 3", got)
	}
	req := &payrollv1.CreateRunRequest{Division: "SOLO", Yymm: 202611, CompanyId: 0, Mode: payrollv1.RunMode_RUN_MODE_DRY}
	out := run.DryResult{Duration: time.Second}
	for _, see := range []bool{false, true} {
		if got := dryRunPB(context.Background(), req, 2, 7, out, see).GetCompanyId(); got != 2 {
			t.Fatalf("dryRunPB(see=%v) company_id = %d, want 2", see, got)
		}
	}
}
