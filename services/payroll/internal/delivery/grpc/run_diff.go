package grpc

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// T030c: GET /runs/{id}/diff (07§4, D-67). The categorization is recon.Diff, the same code as `cmd/recon diff`;
// this file only maps errors, filters/pages the lines and masks nominals without payroll.salary.amount.view (I23).

const keyAgainst = "against"

// RunDiffLoader loads the diff report of a run as the caller (recon.RunDiffSource).
type RunDiffLoader interface {
	Load(ctx context.Context, q recon.RunDiffQuery) (recon.DiffReport, error)
}

// GetRunDiff compares a run with the ACTIVE run, another run of the period, or the LEGACY run (recon).
// A run outside the caller scope is NOT_FOUND, like GetRun (D-53).
func (h *RunHandler) GetRunDiff(ctx context.Context, r *payrollv1.GetRunDiffRequest) (*payrollv1.GetRunDiffResponse, error) {
	if h.diff == nil {
		return nil, status.Error(codes.Unimplemented, "run diff is not configured")
	}
	against := r.GetAgainst()
	if against == "" {
		against = recon.AgainstActive
	}
	rep, err := h.diff.Load(ctx, recon.RunDiffQuery{RunID: r.GetRunId(), Against: against})
	if err != nil {
		return nil, fail(ctx, diffErr(err))
	}
	page, size := int(r.GetPage()), int(r.GetPageSize())
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultRunLinePage
	}
	lines := filterDiffLines(rep.Lines, r.GetNik(), r.GetCategory())
	total := len(lines)
	from := min((page-1)*size, total)
	to := min(from+size, total)
	see := canSeeAmounts(ctx)
	pages := (total + size - 1) / size
	return &payrollv1.GetRunDiffResponse{Base: ok("OK"), Data: runDiffPB(r.GetRunId(), against, rep, lines[from:to], r.GetNik(), see),
		Pagination: &commonv1.PaginationResponse{CurrentPage: int32(page), PageSize: int32(size), TotalItems: int64(total), //nolint:gosec // bounded
			TotalPages: int32(pages)}}, nil //nolint:gosec // bounded by lines
}

// diffErr maps the recon and loader errors to the API codes (07§4). Unknown errors stay internal.
func diffErr(err error) error {
	switch {
	case errors.Is(err, recon.ErrTargetRun):
		// against=legacy compares an ENGINE run with the LEGACY run; a LEGACY run itself is not a valid run_id.
		return apperr.New(apperr.CodeValidation, "against=legacy hanya untuk run ENGINE; run_id ini bukan run ENGINE periode itu",
			map[string]string{keyField: "run_id"})
	case errors.Is(err, recon.ErrBadAgainst), errors.Is(err, recon.ErrAgainstPeriod):
		return apperr.New(apperr.CodeValidation, err.Error(), map[string]string{keyField: keyAgainst})
	case errors.Is(err, recon.ErrRunNotFound):
		return apperr.New(apperr.CodeNotFound, "run tidak ditemukan", nil)
	case errors.Is(err, recon.ErrNoActiveRun), errors.Is(err, recon.ErrNoLegacyRun):
		return apperr.New(apperr.CodeNotFound, err.Error(), map[string]string{keyField: keyAgainst})
	case errors.Is(err, run.ErrRuleIncomplete), errors.Is(err, run.ErrSubsidyBasisPeriods):
		return apperr.New(apperr.CodeRuleIncomplete, err.Error(), nil)
	case errors.Is(err, run.ErrMissingAttdDays):
		return apperr.New(apperr.CodeMissingAttdDays, err.Error(), nil)
	case errors.Is(err, run.ErrMissingAttendance), errors.Is(err, run.ErrNoSnapshot):
		return apperr.New(apperr.CodeMissingAttendance, err.Error(), nil)
	default:
		return err
	}
}

func filterDiffLines(in []recon.LineDiff, nik, category string) []recon.LineDiff {
	if nik == "" && category == "" {
		return in
	}
	out := make([]recon.LineDiff, 0, len(in))
	for _, l := range in {
		if (nik == "" || l.NIK == nik) && (category == "" || l.Category == category) {
			out = append(out, l)
		}
	}
	return out
}

func runDiffPB(runID int64, against string, rep recon.DiffReport, lines []recon.LineDiff, nik string, see bool) *payrollv1.RunDiff {
	out := &payrollv1.RunDiff{RunId: runID, Against: against, AgainstRunId: rep.LegacyRunID, Yymm: int32(rep.YYMM), //nolint:gosec // validated yymm
		Division: rep.Division, Summary: diffSummaryPB(rep.Summary, see), Warnings: warningsPB(rep.Warnings, see)}
	for _, l := range lines {
		out.Lines = append(out.Lines, &payrollv1.RunDiffLine{Nik: l.NIK, Component: l.Component, Category: l.Category,
			LegacyAmount: mask(see, money2(l.Legacy)), TargetAmount: mask(see, money2(l.Target)), DeltaAmount: mask(see, l.Delta.StringFixed(2))})
	}
	for _, hd := range rep.Heads {
		if nik != "" && hd.NIK != nik {
			continue
		}
		pb := &payrollv1.RunDiffHead{Nik: hd.NIK, HasLegacy: hd.Legacy != nil, HasTarget: hd.Target != nil, Excluded: hd.Excluded}
		pb.LegacyTotalTunjangan, pb.LegacyTotalPotongan, pb.LegacyTakeHome = headPB(hd.Legacy, see)
		pb.TargetTotalTunjangan, pb.TargetTotalPotongan, pb.TargetTakeHome = headPB(hd.Target, see)
		out.Heads = append(out.Heads, pb)
	}
	return out
}

func diffSummaryPB(s recon.DiffSummary, see bool) *payrollv1.RunDiffSummary {
	out := &payrollv1.RunDiffSummary{Categories: make(map[string]int32, len(s.Categories)), NikLegacy: int32(s.NIKLegacy), //nolint:gosec // bounded
		NikTarget: int32(s.NIKTarget), HeadMismatch: int32(s.HeadMismatch), Parity: s.Parity, //nolint:gosec // bounded
		TakeHomeLegacy: mask(see, s.TakeHomeLegacy.StringFixed(2)), TakeHomeTarget: mask(see, s.TakeHomeTarget.StringFixed(2)),
		TakeHomeDelta: mask(see, s.TakeHomeDelta.StringFixed(2))}
	for c, n := range s.Categories {
		out.Categories[c] = int32(n) //nolint:gosec // bounded by lines
	}
	for _, reason := range recon.Exclusions {
		x, found := s.Excluded[reason]
		if !found || x.NIK == 0 {
			continue
		}
		out.Excluded = append(out.Excluded, &payrollv1.RunDiffExclusion{Reason: reason, Nik: int32(x.NIK), Lines: int32(x.Lines), //nolint:gosec // bounded
			OtherLines: int32(x.OtherLines), HeadMismatch: int32(x.HeadMismatch), //nolint:gosec // bounded
			TakeHomeLegacy: mask(see, x.TakeHomeLegacy.StringFixed(2)), TakeHomeTarget: mask(see, x.TakeHomeTarget.StringFixed(2)),
			TakeHomeDelta: mask(see, x.TakeHomeDelta.StringFixed(2))})
	}
	return out
}

// headPB returns tunjangan, potongan and take-home; a missing side is "" (masked like any nominal).
func headPB(h *recon.HeadTotals, see bool) (string, string, string) {
	if h == nil {
		return mask(see, ""), mask(see, ""), mask(see, "")
	}
	return mask(see, h.TotTunjangan.StringFixed(2)), mask(see, h.TotPotongan.StringFixed(2)), mask(see, h.TakeHome.StringFixed(2))
}

// money2 formats a nullable amount; nil (no line or NULL, D10) is "".
func money2(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return d.StringFixed(2)
}
