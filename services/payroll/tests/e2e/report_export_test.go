package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	auditdomain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T030d: ExportReport (07§6) against PG18 as payroll_app. The workbook of the RPC must equal the output of the code
// path of `cmd/report reco|bank` (ExportReco/ExportBank + WriteReco/WriteBank) for the same synthetic data; every
// export writes exactly one REPORT_EXPORT row; both permissions are required; BANK is refused while the period is OPEN.
const (
	expYYMM     = 203703
	expOpenYYMM = 203704
)

var (
	permsExport       = []string{"payroll.report.data.view", "payroll.salary.amount.view"}
	permsExportNoAmnt = []string{"payroll.report.data.view"}
)

func expText(td time.Time) (report.BankText, error) {
	return report.BankText{CompanyName: "PT. X", Branch: "SOLO", EscrowAccount: "9999", Signer1: "S1", Title1: "T1", Signer2: "S2",
		Title2: "T2", FeePerTransfer: decimal.NewFromInt(2200), TransferDate: td, PeriodDate: td}, nil
}

func seedExportEmployees(t *testing.T) {
	t.Helper()
	q := `INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level, pe_cost_group, pe_bank, pe_bank_acnt_name, pe_bank_acnt_enc)
	 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','Executive','MGT','Bank Mandiri','Rep Satu', payroll.pgp_sym_encrypt('1234567890','` + encKey + `')),
	        (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','Executive','MGT','Bank BCA','Rep Dua', payroll.pgp_sym_encrypt('9876543210','` + encKey + `'))
	 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_bank = EXCLUDED.pe_bank, pe_bank_acnt_name = EXCLUDED.pe_bank_acnt_name,
	   pe_bank_acnt_enc = EXCLUDED.pe_bank_acnt_enc, pe_level = EXCLUDED.pe_level, pe_cost_group = EXCLUDED.pe_cost_group`
	if _, err := ownDB.Exec(q); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik IN ('ZREP0001','ZREP0002')`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
}

func exportAudit(t *testing.T, yymm int, kind string) string {
	t.Helper()
	return scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after->>'jenis'='`+kind+
		`' AND pal_after->>'yymm'='`+itoa(yymm)+`' AND pal_actor='`+userRW+`'`)
}

// sameWorkbook compares two xlsx files sheet by sheet: order, names, cell values and formulas.
func sameWorkbook(t *testing.T, name string, a, b []byte) {
	t.Helper()
	fa, err := excelize.OpenReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	fb, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	sa, sb := fa.GetSheetList(), fb.GetSheetList()
	if len(sa) != len(sb) {
		t.Fatalf("%s: sheets %v vs %v", name, sa, sb)
	}
	for i, sh := range sa {
		if sb[i] != sh {
			t.Fatalf("%s: sheet %d %q vs %q", name, i, sh, sb[i])
		}
		ra, _ := fa.GetRows(sh, excelize.Options{RawCellValue: true}) //nolint:errcheck // compared below
		rb, _ := fb.GetRows(sh, excelize.Options{RawCellValue: true}) //nolint:errcheck // compared below
		if len(ra) == 0 && sh != report.SheetPreCheck {
			t.Fatalf("%s: sheet %q is empty", name, sh)
		}
		if len(ra) != len(rb) {
			t.Fatalf("%s: sheet %q rows %d vs %d", name, sh, len(ra), len(rb))
		}
		for r := range ra {
			if len(ra[r]) != len(rb[r]) {
				t.Fatalf("%s: sheet %q row %d width", name, sh, r+1)
			}
			for c := range ra[r] {
				if ra[r][c] != rb[r][c] {
					t.Fatalf("%s: sheet %q r%dc%d %q vs %q", name, sh, r+1, c+1, ra[r][c], rb[r][c])
				}
				cell, _ := excelize.CoordinatesToCellName(c+1, r+1) //nolint:errcheck // valid coordinates
				fma, _ := fa.GetCellFormula(sh, cell)               //nolint:errcheck // compared
				fmb, _ := fb.GetCellFormula(sh, cell)               //nolint:errcheck // compared
				if fma != fmb {
					t.Fatalf("%s: sheet %q %s formula %q vs %q", name, sh, cell, fma, fmb)
				}
			}
		}
	}
}

func TestExportReport(t *testing.T) {
	seedReportRun(t, expYYMM, "APPROVED")
	seedExportEmployees(t)
	svc := report.NewService(appDB, auditinfra.NewWriter())
	// Own handler (not runHandler): its once-only employee isolation must not run before the other report tests seed theirs.
	h := grpcdelivery.NewRunHandler(nil, nil, postgres.NewRunQuery(appDB)).WithReports(svc, encKey, expText)
	solo := peer.NewContext(ctxFor(t, userRW, permsExport...), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.9.8.7"), Port: 4242}})
	q := report.Query{CompanyID: 1, YYMM: expYYMM, Division: "SOLO"}
	td := time.Date(2037, 3, 26, 0, 0, 0, 0, time.UTC)

	// RECO: equals the cmd/report reco path; one audit row per export.
	var want bytes.Buffer
	if err := svc.ExportReco(solo, report.Query{CompanyID: 1, YYMM: expYYMM, Division: "SOLO", AllowOpen: true}, encKey,
		func(d report.RecoData) (int, error) { return report.WriteReco(d, &want) }); err != nil {
		t.Fatal(err)
	}
	if got := exportAudit(t, expYYMM, "reco"); got != "1" {
		t.Fatalf("direct reco audit = %s", got)
	}
	resp, err := h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetFileName() != "reco_SOLO_203703.xlsx" || len(resp.GetFileContent()) == 0 || resp.GetRowCount() == 0 {
		t.Fatalf("reco resp: name %q bytes %d rows %d", resp.GetFileName(), len(resp.GetFileContent()), resp.GetRowCount())
	}
	sameWorkbook(t, "reco", resp.GetFileContent(), want.Bytes())
	if got := exportAudit(t, expYYMM, "reco"); got != "2" {
		t.Fatalf("reco audit after RPC = %s, want 2 (1 direct + 1 RPC)", got)
	}
	// The API row carries actor, company, division, IP and the real period status; APPROVED needs no override, so
	// allow_open is false although the RECO RPC runs with the rehearsal policy. The direct (CLI-path) row has no IP.
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_actor='`+userRW+
		`' AND pal_company_id=1 AND pal_division='SOLO' AND host(pal_ip)='10.9.8.7' AND pal_after->>'jenis'='reco' AND pal_after->>'yymm'='`+itoa(expYYMM)+
		`' AND pal_after->>'period_status'='APPROVED' AND pal_after->>'allow_open'='false'`); got != "1" {
		t.Fatalf("reco API audit row = %s, want 1", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_ip IS NULL AND pal_after->>'jenis'='reco' AND pal_after->>'yymm'='`+itoa(expYYMM)+`'`); got != "1" {
		t.Fatalf("CLI-path audit row must keep an empty IP: %s", got)
	}

	// BANK: equals the cmd/report bank path (ExportBank + WriteBank); the pre-check leaves ZREP0002 out (BCA).
	text, _ := expText(td) //nolint:errcheck // never fails
	var wantBank bytes.Buffer
	var wantRows int
	if err := svc.ExportBank(solo, q, encKey, report.MandiriBank, func(d report.BankData) (int, error) {
		n, err := report.WriteBank(d, text, &wantBank)
		wantRows = n
		return n, err
	}); err != nil {
		t.Fatal(err)
	}
	bank, err := h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-03-26"})
	if err != nil {
		t.Fatal(err)
	}
	if bank.GetFileName() != "bank_SOLO_203703.xlsx" || int(bank.GetRowCount()) != wantRows || bank.GetSkippedCount() != 1 {
		t.Fatalf("bank resp: name %q rows %d skipped %d", bank.GetFileName(), bank.GetRowCount(), bank.GetSkippedCount())
	}
	sameWorkbook(t, "bank", bank.GetFileContent(), wantBank.Bytes())
	if got := exportAudit(t, expYYMM, "bank"); got != "2" {
		t.Fatalf("bank audit = %s, want 2 (1 direct + 1 RPC)", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_actor='`+userRW+
		`' AND pal_company_id=1 AND pal_division='SOLO' AND host(pal_ip)='10.9.8.7' AND pal_after->>'jenis'='bank' AND pal_after->>'bank'='MANDIRI'`+
		` AND pal_after->>'transfer_date'='2037-03-26' AND pal_after->>'period_status'='APPROVED' AND pal_after->>'yymm'='`+itoa(expYYMM)+`'`); got != "1" {
		t.Fatalf("bank API audit row = %s, want 1", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND (pal_after::text LIKE '%ZREP%' OR pal_after::text LIKE '%1234567890%')`); got != "0" {
		t.Fatal("audit holds NIK or account number")
	}

	// 403 without either permission: nothing built, nothing audited.
	before := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'`)
	_, err = h.ExportReport(ctxFor(t, userRW, permsExportNoAmnt...), &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO})
	wantCode(t, "no salary.amount.view", err, apperr.CodePermissionDenied)
	// The interceptor maps ExportReport to payroll.report.data.view (TestRunRPCsHavePermissions); the handler itself
	// enforces salary.amount.view, so a caller with neither is refused as well.
	_, err = h.ExportReport(ctxFor(t, userRW), &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-03-26"})
	wantCode(t, "no permission at all", err, apperr.CodePermissionDenied)
	// Scope: a JAKARTA-only user cannot export a SOLO period.
	_, err = h.ExportReport(ctxFor(t, userJkt, permsExport...), &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO})
	wantCode(t, "other division", err, apperr.CodeDivisionForbidden)
	// BANK needs a transfer date.
	_, err = h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK})
	wantCode(t, "bank without date", err, apperr.CodeValidation)
	if after := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'`); after != before {
		t.Fatalf("refused exports audited: %s -> %s", before, after)
	}

	// BANK is refused while the period is OPEN (422 BUSINESS_RULE), RECO (rehearsal) is allowed and audited allow_open.
	seedReportRun(t, expOpenYYMM, "OPEN")
	_, err = h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expOpenYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-04-26"})
	wantCode(t, "bank on OPEN period", err, apperr.CodeBusinessRule)
	if got := exportAudit(t, expOpenYYMM, "bank"); got != "0" {
		t.Fatalf("refused BANK audited: %s", got)
	}
	if _, err = h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expOpenYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO}); err != nil {
		t.Fatalf("reco on OPEN: %v", err)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after->>'yymm'='`+itoa(expOpenYYMM)+
		`' AND pal_after->>'allow_open'='true' AND pal_after->>'period_status'='OPEN' AND host(pal_ip)='10.9.8.7'`); got != "1" {
		t.Fatalf("open reco audit = %s", got)
	}
	_, err = h.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: 209901, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO})
	wantCode(t, "unknown period", err, apperr.CodeNotFound)

	// A bank text (PAYROLL_REPORT_*) failure is INTERNAL without details, and the period check runs first: an OPEN period
	// is still BUSINESS_RULE although the config is broken. Nothing is audited for either.
	badText := func(time.Time) (report.BankText, error) {
		return report.BankText{}, errors.New("config: wajib diisi: PAYROLL_REPORT_COMPANY_NAME")
	}
	hb := grpcdelivery.NewRunHandler(nil, nil, postgres.NewRunQuery(appDB)).WithReports(svc, encKey, badText)
	_, err = hb.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expOpenYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-04-26"})
	wantCode(t, "bank on OPEN with broken config", err, apperr.CodeBusinessRule)
	pre := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'`)
	_, err = hb.ExportReport(solo, &payrollv1.ExportReportRequest{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-03-26"})
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "PAYROLL_REPORT") {
		t.Fatalf("broken bank config: %v, want INTERNAL without details", err)
	}
	if post := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'`); post != pre {
		t.Fatalf("config failure audited: %s -> %s", pre, post)
	}

	// An audit writer failure rolls the export back: no file and no audit row come out.
	hf := grpcdelivery.NewRunHandler(nil, nil, postgres.NewRunQuery(appDB)).WithReports(report.NewService(appDB, failingAudit{}), encKey, expText)
	for _, req := range []*payrollv1.ExportReportRequest{
		{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_RECO},
		{Yymm: expYYMM, Division: "SOLO", Kind: payrollv1.ReportKind_REPORT_KIND_BANK, TransferDate: "2037-03-26"},
	} {
		r, err := hf.ExportReport(solo, req)
		if err == nil || r != nil {
			t.Fatalf("%v with failing audit: resp=%v err=%v, want error and no file", req.GetKind(), r, err)
		}
	}
	if post := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'`); post != pre {
		t.Fatalf("failed audit left rows: %s -> %s", pre, post)
	}
}

// failingAudit is an audit.Writer whose Write always fails.
type failingAudit struct{}

func (failingAudit) Write(context.Context, *sql.Tx, auditdomain.Entry) error {
	return errors.New("audit: boom")
}
