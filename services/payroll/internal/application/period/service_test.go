package period

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

func TestBlockingWarnings(t *testing.T) {
	cmd := Command{CompanyID: 1, Division: "SOLO", YYMM: 202610}
	tests := []struct {
		name     string
		warnings sql.NullString
		wantErr  bool
		wantMsg  string
	}{
		{"no warnings", sql.NullString{}, false, ""},
		{"only NO_WORKDAYS passes", sql.NullString{String: `[{"nik":"1","code":"NO_WORKDAYS"}]`, Valid: true}, false, ""},
		{"PPH_NO_PTKP and PPH_NO_BRACKET block (T033i)", sql.NullString{String: `[{"nik":"1","code":"PPH_NO_PTKP"},{"nik":"2","code":"PPH_NO_BRACKET"},{"nik":"3","code":"YTD_NO_NPWP"}]`, Valid: true}, true, "2 karyawan"},
		{"calc error blocks", sql.NullString{String: `[{"nik":"1","code":"CALC_ERROR"},{"nik":"1","code":"LOAN_DOUBLE"},{"nik":"2","code":"INCOMPLETE_RESULT"}]`, Valid: true}, true, "2 karyawan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := blockingWarnings(cmd, state{warnings: tt.warnings})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantMsg)
			}
		})
	}
}

func TestBlockingWarningsBadJSON(t *testing.T) {
	if err := blockingWarnings(Command{}, state{warnings: sql.NullString{String: `{`, Valid: true}}); err == nil {
		t.Fatal("want parse error")
	}
}

func TestBlockingWarningsCapsNIKList(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < maxBlockedNIKs+5; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"nik":"N` + string(rune('A'+i)) + `","code":"CALC_ERROR"}`)
	}
	sb.WriteString("]")
	err := blockingWarnings(Command{}, state{warnings: sql.NullString{String: sb.String(), Valid: true}})
	if err == nil {
		t.Fatal("want error")
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		if n := len(strings.Split(ae.Details["niks"], ",")); n != maxBlockedNIKs {
			t.Fatalf("niks listed = %d, want %d", n, maxBlockedNIKs)
		}
	}
}

func TestCheckInput(t *testing.T) {
	s := &Service{}
	cmd := Command{CompanyID: 1, Division: "SOLO", YYMM: 202610}
	// LEGACY and run-less periods are not checked, even without a hasher.
	for _, st := range []state{{runKind: sql.NullString{String: "LEGACY", Valid: true}}, {}} {
		if err := s.checkInput(nil, nil, cmd, st); err != nil { //nolint:staticcheck // ctx unused on these paths
			t.Fatalf("unexpected error: %v", err)
		}
	}
	st := state{runKind: sql.NullString{String: "ENGINE", Valid: true}, runID: sql.NullInt64{Int64: 5, Valid: true}}
	if err := s.checkInput(nil, nil, cmd, st); err == nil { //nolint:staticcheck // ctx unused on this path
		t.Fatal("ENGINE run without hasher must error")
	}
}

// T033i: the engine PPh warning texts parse (run.engineWarning) to codes that block approve.
func TestBlockingCodesCoverPPhWarnings(t *testing.T) {
	for _, c := range []string{run.CodePPhNoPTKP, run.CodePPhNoBracket, run.CodeCalcError, run.CodeLoanDouble, run.CodeIncomplete} {
		if !blockingCodes[c] {
			t.Errorf("%s not blocking", c)
		}
	}
	for _, w := range []string{engine.WarnPPhNoPTKP + `: 1 status pajak "K/1"`, engine.WarnPPhNoBracket + `: 1 status pajak "", bruto 5000000`} {
		if is := run.ParseEngineWarning("1", w); !blockingCodes[is.Code] {
			t.Errorf("warning %q → code %q, not blocking", w, is.Code)
		}
	}
}
