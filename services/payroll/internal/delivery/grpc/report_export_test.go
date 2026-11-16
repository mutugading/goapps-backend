package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

type fakeExporter struct {
	calls int
	err   error
	q     report.Query
}

func (f *fakeExporter) RecoFile(_ context.Context, q report.Query, _ string) (report.File, error) {
	f.calls++
	f.q = q
	return report.File{Name: "reco.xlsx", Content: []byte("x"), Rows: 3, Skipped: 2, SkippedNoHead: 9}, f.err
}

func (f *fakeExporter) BankFile(_ context.Context, q report.Query, _, _ string, _ time.Time, _ report.TextLoader) (report.File, error) {
	f.calls++
	f.q = q
	return report.File{Name: "bank.xlsx", Content: []byte("x"), Rows: 2, Skipped: 1, SkippedNoHead: 9}, f.err
}

func TestExportReport_RejectsBeforeBuilding(t *testing.T) {
	ex := &fakeExporter{}
	text := func(time.Time) (report.BankText, error) { return report.BankText{}, nil }
	h := (&RunHandler{}).WithReports(ex, "k", text)
	only := context.WithValue(context.Background(), AuthPermissionsKey, []string{permReportView})
	_, err := h.ExportReport(only, &payrollv1.ExportReportRequest{Yymm: 202701, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO})
	if c, _ := CodeFromError(err); c != apperr.CodePermissionDenied {
		t.Fatalf("without salary.amount.view: %v", err)
	}
	both := context.WithValue(context.Background(), AuthPermissionsKey, []string{permReportView, permSalaryAmount})
	for name, r := range map[string]*payrollv1.ExportReportRequest{
		"no kind":        {Yymm: 202701, Division: "SOLO"},
		"bank, no date":  {Yymm: 202701, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK},
		"bank, bad date": {Yymm: 202701, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "26-01-2027"},
	} {
		if _, err := h.ExportReport(both, r); func() apperr.Code { c, _ := CodeFromError(err); return c }() != apperr.CodeValidation {
			t.Errorf("%s: %v", name, err)
		}
	}
	if ex.calls != 0 {
		t.Fatalf("exporter called %d times for refused requests", ex.calls)
	}
	if _, err := (&RunHandler{}).ExportReport(both, &payrollv1.ExportReportRequest{}); err == nil {
		t.Fatal("unconfigured handler must fail")
	}
}

func TestReportErr(t *testing.T) {
	for _, c := range []struct {
		in   error
		want apperr.Code
	}{
		{report.ErrPeriodNotApproved, apperr.CodeBusinessRule},
		{report.ErrNoPayableRows, apperr.CodeBusinessRule},
		{report.ErrNoRun, apperr.CodeNotFound},
		{fmt.Errorf("%w: %w", report.ErrBankConfig, errors.New("config: wajib diisi: X")), apperr.CodeBusinessRule},
	} {
		var ae *apperr.Error
		if got := reportErr(c.in); !errors.As(got, &ae) || ae.Code != c.want {
			t.Errorf("%v -> %v, want %s", c.in, got, c.want)
		}
	}
	other := errors.New("boom")
	if reportErr(other) != other {
		t.Fatal("unknown errors must pass through")
	}
}

type stubReader struct{ run.Reader }

func (stubReader) GetPeriod(context.Context, int16, int, string) (run.PeriodInfo, error) {
	return run.PeriodInfo{Status: "APPROVED"}, nil
}

// An exporter failure (audit write, period check, config) hands out no file and no partial response, and the audit
// IP of the caller is passed on in the query.
func TestExportReport_ErrorYieldsNoFile(t *testing.T) {
	sc, err := scope.New(1, "SOLO")
	if err != nil {
		t.Fatal(err)
	}
	ctx := scope.WithIdentity(context.Background(), scope.Identity{UserID: "u", Scopes: []scope.Scope{sc}})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{permReportView, permSalaryAmount})
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 5555}})
	text := func(time.Time) (report.BankText, error) { return report.BankText{}, nil }
	for _, kind := range []payrollv1.ReportKind{payrollv1.ReportKind_REPORT_KIND_RECO, payrollv1.ReportKind_REPORT_KIND_BANK} {
		ex := &fakeExporter{err: errors.New("audit: failed to write audit log")}
		h := (&RunHandler{reader: stubReader{}}).WithReports(ex, "k", text)
		resp, err := h.ExportReport(ctx, &payrollv1.ExportReportRequest{Yymm: 202701, Division: "SOLO", Kind: kind, TransferDate: "2027-01-26"})
		if err == nil || resp != nil {
			t.Fatalf("kind %v: resp=%v err=%v, want error and no response", kind, resp, err)
		}
		if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "audit") {
			t.Fatalf("kind %v: unknown errors must be INTERNAL without details, got %v", kind, err)
		}
		if ex.q.IP != "10.1.2.3" || ex.q.CompanyID != 1 {
			t.Fatalf("kind %v: query = %+v, want IP and company passed on", kind, ex.q)
		}
	}
}

// The count of NIKs without a head (D-70) travels in its own field, apart from skipped_count (BANK: pre-check, RECO:
// components without GL).
func TestExportReport_SkippedNoHeadCount(t *testing.T) {
	sc, err := scope.New(1, "SOLO")
	if err != nil {
		t.Fatal(err)
	}
	ctx := scope.WithIdentity(context.Background(), scope.Identity{UserID: "u", Scopes: []scope.Scope{sc}})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{permReportView, permSalaryAmount})
	text := func(time.Time) (report.BankText, error) { return report.BankText{}, nil }
	for kind, wantSkipped := range map[payrollv1.ReportKind]int32{payrollv1.ReportKind_REPORT_KIND_RECO: 2, payrollv1.ReportKind_REPORT_KIND_BANK: 1} {
		h := (&RunHandler{reader: stubReader{}}).WithReports(&fakeExporter{}, "k", text)
		resp, err := h.ExportReport(ctx, &payrollv1.ExportReportRequest{Yymm: 202701, Division: "SOLO", Kind: kind, TransferDate: "2027-01-26"})
		if err != nil {
			t.Fatalf("kind %v: %v", kind, err)
		}
		if resp.GetSkippedNoHeadCount() != 9 || resp.GetSkippedCount() != wantSkipped {
			t.Errorf("kind %v: no_head=%d skipped=%d, want 9 and %d", kind, resp.GetSkippedNoHeadCount(), resp.GetSkippedCount(), wantSkipped)
		}
	}
}
