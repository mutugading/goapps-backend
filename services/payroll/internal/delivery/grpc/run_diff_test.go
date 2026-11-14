package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/shopspring/decimal"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

type fakeDiff struct {
	rep recon.DiffReport
	err error
	got recon.RunDiffQuery
}

func (f *fakeDiff) Load(_ context.Context, q recon.RunDiffQuery) (recon.DiffReport, error) {
	f.got = q
	return f.rep, f.err
}

func decp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func sampleDiff() recon.DiffReport {
	rep := recon.DiffReport{DiffMeta: recon.DiffMeta{YYMM: 203501, Division: "SOLO", LegacyRunID: 7}}
	rep.Summary.Categories = map[string]int{recon.CatMatch: 3, recon.CatFormula: 2, recon.CatOnlyTarget: 1}
	rep.Summary.TakeHomeLegacy, rep.Summary.TakeHomeTarget = decimal.RequireFromString("100"), decimal.RequireFromString("150.5")
	rep.Summary.TakeHomeDelta = decimal.RequireFromString("50.5")
	rep.Summary.Excluded = map[string]recon.ExclusionStat{recon.ExclMasterDrift: {NIK: 1, Lines: 1, TakeHomeDelta: decimal.RequireFromString("9")}}
	rep.Lines = []recon.LineDiff{
		{NIK: "N1", Component: "T001", Legacy: decp("10"), Target: decp("20"), Delta: decimal.RequireFromString("10"), Category: recon.CatFormula},
		{NIK: "N2", Component: "T001", Legacy: decp("10"), Target: decp("30"), Delta: decimal.RequireFromString("20"), Category: recon.CatFormula},
		{NIK: "N2", Component: "P005", Target: decp("5"), Delta: decimal.RequireFromString("5"), Category: recon.CatOnlyTarget},
	}
	rep.Heads = []recon.HeadDiff{{NIK: "N2", Target: &recon.HeadTotals{TotTunjangan: decimal.RequireFromString("30"), TakeHome: decimal.RequireFromString("25")}}}
	rep.Warnings = []run.Issue{{NIK: "N9", Code: recon.CodeExclMasterDrift, Message: "net 1234567"}}
	return rep
}

func TestGetRunDiff_PagingFilterAndDefaultAgainst(t *testing.T) {
	f := &fakeDiff{rep: sampleDiff()}
	h := (&RunHandler{}).WithDiff(f)
	ctx := context.WithValue(context.Background(), AuthPermissionsKey, []string{permSalaryAmount})
	resp, err := h.GetRunDiff(ctx, &payrollv1.GetRunDiffRequest{RunId: 5, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if f.got.Against != recon.AgainstActive || f.got.RunID != 5 {
		t.Fatalf("query = %+v, want against=active run 5", f.got)
	}
	d := resp.GetData()
	if len(d.GetLines()) != 2 || resp.GetPagination().GetTotalItems() != 3 || resp.GetPagination().GetTotalPages() != 2 {
		t.Fatalf("page 1: lines %d pagination %v", len(d.GetLines()), resp.GetPagination())
	}
	if l := d.GetLines()[0]; l.GetLegacyAmount() != "10.00" || l.GetTargetAmount() != "20.00" || l.GetDeltaAmount() != "10.00" {
		t.Fatalf("line = %v", l)
	}
	if d.GetAgainstRunId() != 7 || d.GetSummary().GetCategories()[recon.CatFormula] != 2 || d.GetSummary().GetTakeHomeDelta() != "50.50" ||
		len(d.GetSummary().GetExcluded()) != 1 || d.GetSummary().GetExcluded()[0].GetTakeHomeDelta() != "9.00" {
		t.Fatalf("summary = %v", d.GetSummary())
	}
	if hd := d.GetHeads()[0]; hd.GetHasLegacy() || !hd.GetHasTarget() || hd.GetLegacyTakeHome() != "" || hd.GetTargetTakeHome() != "25.00" {
		t.Fatalf("head = %v", hd)
	}
	p2, err := h.GetRunDiff(ctx, &payrollv1.GetRunDiffRequest{RunId: 5, Against: "run:3", PageSize: 2, Page: 2})
	if err != nil || len(p2.GetData().GetLines()) != 1 || p2.GetData().GetLines()[0].GetLegacyAmount() != "" || f.got.Against != "run:3" {
		t.Fatalf("page 2: %v %v (query %+v)", p2, err, f.got)
	}
	flt, err := h.GetRunDiff(ctx, &payrollv1.GetRunDiffRequest{RunId: 5, Nik: "N2", Category: recon.CatOnlyTarget})
	if err != nil || len(flt.GetData().GetLines()) != 1 || flt.GetData().GetLines()[0].GetComponent() != "P005" || len(flt.GetData().GetHeads()) != 1 {
		t.Fatalf("filter: %v %v", flt, err)
	}
	none, err := h.GetRunDiff(ctx, &payrollv1.GetRunDiffRequest{RunId: 5, Nik: "N1"})
	if err != nil || len(none.GetData().GetHeads()) != 0 {
		t.Fatalf("nik filter must drop other heads: %v %v", none, err)
	}
	beyond, err := h.GetRunDiff(ctx, &payrollv1.GetRunDiffRequest{RunId: 5, Page: 9})
	if err != nil || len(beyond.GetData().GetLines()) != 0 {
		t.Fatalf("page beyond end: %v %v", beyond, err)
	}
}

func TestGetRunDiff_Masked(t *testing.T) {
	h := (&RunHandler{}).WithDiff(&fakeDiff{rep: sampleDiff()})
	resp, err := h.GetRunDiff(context.Background(), &payrollv1.GetRunDiffRequest{RunId: 5})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	walkNominals(resp.ProtoReflect(), "GetRunDiff", func(path, v string, _ bool) {
		seen++
		if v != masked {
			t.Errorf("%s = %q, want %q", path, v, masked)
		}
	})
	if seen == 0 {
		t.Fatal("no nominal field found")
	}
	d := resp.GetData()
	if d.GetLines()[0].GetCategory() != recon.CatFormula || d.GetLines()[0].GetNik() != "N1" || d.GetSummary().GetCategories()[recon.CatMatch] != 3 {
		t.Fatalf("masked response lost non-nominal fields: %v", d)
	}
	if w := d.GetWarnings(); len(w) != 1 || w[0] != fmt.Sprintf(runWarningFmtNIK, recon.CodeExclMasterDrift, "N9", masked) {
		t.Fatalf("warnings = %v", w)
	}
}

func TestGetRunDiff_Errors(t *testing.T) {
	if _, err := (&RunHandler{}).GetRunDiff(context.Background(), &payrollv1.GetRunDiffRequest{RunId: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("no source: %v", err)
	}
	for _, tc := range []struct {
		err  error
		want apperr.Code
	}{
		{fmt.Errorf("x: %w", recon.ErrRunNotFound), apperr.CodeNotFound},
		{recon.ErrNoActiveRun, apperr.CodeNotFound},
		{recon.ErrNoLegacyRun, apperr.CodeNotFound},
		{recon.ErrBadAgainst, apperr.CodeValidation},
		{recon.ErrAgainstPeriod, apperr.CodeValidation},
		{fmt.Errorf("load rules: %w", run.ErrRuleIncomplete), apperr.CodeRuleIncomplete},
		{fmt.Errorf("load inputs: %w", run.ErrMissingAttendance), apperr.CodeMissingAttendance},
		{apperr.DivisionForbidden(1, "JAKARTA"), apperr.CodeDivisionForbidden},
	} {
		h := (&RunHandler{}).WithDiff(&fakeDiff{err: tc.err})
		_, err := h.GetRunDiff(context.Background(), &payrollv1.GetRunDiffRequest{RunId: 1})
		if got, found := CodeFromError(err); !found || got != tc.want {
			t.Errorf("%v: code = %v (%v), want %s", tc.err, got, err, tc.want)
		}
	}
	h := (&RunHandler{}).WithDiff(&fakeDiff{err: errors.New("boom")})
	if _, err := h.GetRunDiff(context.Background(), &payrollv1.GetRunDiffRequest{RunId: 1}); status.Code(err) != codes.Internal {
		t.Fatalf("unknown error: %v", err)
	}
}

// The REST route has no ?against= by default: the empty value must pass protovalidate so the handler default
// (active) is reachable; unknown values are still refused.
func TestGetRunDiffRequestValidation(t *testing.T) {
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"", "active", "legacy", "run:12"} {
		if err := v.Validate(&payrollv1.GetRunDiffRequest{RunId: 1, Against: a}); err != nil {
			t.Errorf("against %q refused: %v", a, err)
		}
	}
	for _, a := range []string{"dry", "run:0", "run:x", "ACTIVE", " active"} {
		if err := v.Validate(&payrollv1.GetRunDiffRequest{RunId: 1, Against: a}); err == nil {
			t.Errorf("against %q accepted", a)
		}
	}
}

func TestGetRunDiff_LegacyRunIDField(t *testing.T) {
	h := (&RunHandler{}).WithDiff(&fakeDiff{err: fmt.Errorf("x: %w", recon.ErrTargetRun)})
	_, err := h.GetRunDiff(context.Background(), &payrollv1.GetRunDiffRequest{RunId: 1, Against: recon.AgainstLegacy})
	field := ""
	if st, found := status.FromError(err); found {
		for _, d := range st.Details() {
			if info, isInfo := d.(*errdetails.ErrorInfo); isInfo {
				field = info.GetMetadata()[keyField]
			}
		}
	}
	if c, _ := CodeFromError(err); c != apperr.CodeValidation || field != "run_id" || !strings.Contains(err.Error(), "ENGINE") {
		t.Fatalf("legacy on non-ENGINE run = %v, want VALIDATION on run_id", err)
	}
}
