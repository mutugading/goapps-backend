package grpc

import (
	"context"
	"errors"
	"time"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

// T030d: GET /reports/export (07§6, D-67 butir 1, D-62 butir 3/7). The workbooks are built by the same
// report.Service code as `cmd/report reco|bank`; this file only checks permission/scope/period, maps errors and
// hands the bytes out after the audit row (REPORT_EXPORT) committed. There is no masked variant (I23): the files hold
// nominals and bank accounts, so payroll.salary.amount.view is required next to payroll.report.data.view.

const (
	keyKind         = "kind"
	keyTransferDate = "transfer_date"
	kindReco        = "reco"
	kindBank        = "bank"
)

// ReportExporter builds the report workbooks in memory (report.Service).
type ReportExporter interface {
	RecoFile(ctx context.Context, q report.Query, piiKey string) (report.File, error)
	BankFile(ctx context.Context, q report.Query, piiKey, bank string, transferDate time.Time, load report.TextLoader) (report.File, error)
}

// WithReports wires the report export (T030d); without it ExportReport answers UNIMPLEMENTED.
func (h *RunHandler) WithReports(e ReportExporter, piiKey string, text report.TextLoader) *RunHandler {
	h.reports, h.piiKey, h.bankText = e, piiKey, text
	return h
}

// ExportReport returns the reco or bank workbook of a period. Order: permission (interceptor: report.data.view) →
// salary.amount.view → request validation → scope/period lookup → service (period status, audit).
func (h *RunHandler) ExportReport(ctx context.Context, r *payrollv1.ExportReportRequest) (*payrollv1.ExportReportResponse, error) {
	if h.reports == nil {
		return nil, fail(ctx, apperr.New(apperr.CodeInternal, "report export is not configured", nil))
	}
	if !canSeeAmounts(ctx) {
		return nil, fail(ctx, apperr.New(apperr.CodePermissionDenied, "permission denied: requires "+permSalaryAmount, nil))
	}
	kind := r.GetKind()
	if kind != payrollv1.ReportKind_REPORT_KIND_RECO && kind != payrollv1.ReportKind_REPORT_KIND_BANK {
		return nil, fail(ctx, apperr.New(apperr.CodeValidation, "kind wajib RECO atau BANK", map[string]string{keyField: keyKind}))
	}
	var tdate time.Time
	if kind == payrollv1.ReportKind_REPORT_KIND_BANK {
		var err error
		if tdate, err = time.Parse(dateLayout, r.GetTransferDate()); err != nil {
			return nil, fail(ctx, apperr.New(apperr.CodeValidation, "transfer_date wajib (YYYY-MM-DD) untuk BANK", map[string]string{keyField: keyTransferDate}))
		}
	}
	cmd, err := h.periodCommand(ctx, r.GetYymm(), r.GetDivision(), "")
	if err != nil {
		return nil, fail(ctx, err)
	}
	q := report.Query{CompanyID: cmd.CompanyID, YYMM: cmd.YYMM, Division: cmd.Division, IP: cmd.IP}
	var f report.File
	if kind == payrollv1.ReportKind_REPORT_KIND_BANK {
		f, err = h.reports.BankFile(ctx, q, h.piiKey, report.MandiriBank, tdate, h.bankText)
	} else {
		q.AllowOpen = true // the reco workbook is also a rehearsal tool (T028b); the audit row records allow_open and period_status
		f, err = h.reports.RecoFile(ctx, q, h.piiKey)
	}
	if err != nil {
		return nil, fail(ctx, reportErr(err))
	}
	return &payrollv1.ExportReportResponse{Base: ok("OK"), FileContent: f.Content, FileName: f.Name,
		RowCount: int32(f.Rows), SkippedCount: int32(f.Skipped), //nolint:gosec // bounded by employees
		SkippedNoHeadCount: int32(f.SkippedNoHead)}, nil //nolint:gosec // bounded by employees
}

// reportErr maps the report errors to the API codes (07§1.3). Unknown errors stay internal (logged by fail).
func reportErr(err error) error {
	switch {
	case errors.Is(err, report.ErrPeriodNotApproved):
		return apperr.New(apperr.CodeBusinessRule, err.Error(), nil)
	case errors.Is(err, report.ErrNoRun):
		return apperr.New(apperr.CodeNotFound, err.Error(), nil)
	case errors.Is(err, report.ErrNoPayableRows), errors.Is(err, report.ErrBankConfig):
		return apperr.New(apperr.CodeBusinessRule, err.Error(), nil)
	default:
		return err
	}
}
