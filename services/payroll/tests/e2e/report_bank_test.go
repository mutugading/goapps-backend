package e2e

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

// T028c: bank file (F-1) and reco `transfer` sheet (F-2) against PG18 as payroll_app: Σ DETAIL = Σ net_pay = cover total
// = Σ transfer, formulas stay formulas, pre-check, and the account number never reaches the audit log.
const bankYYMM = 203612

func TestReport_Bank(t *testing.T) {
	seedReportRun(t, bankYYMM, "APPROVED")
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_level, pe_cost_group, pe_bank, pe_bank_acnt_name, pe_bank_acnt_enc)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','Executive','MGT','Bank Mandiri','Rep Satu', payroll.pgp_sym_encrypt('1234567890','` + encKey + `')),
		        (1,'ZREP0002','SOLO','Rep Dua','ZREP0002','Executive','MGT','Bank BCA','Rep Dua', payroll.pgp_sym_encrypt('9876543210','` + encKey + `'))
		 ON CONFLICT (pe_company_id, pe_nik, pe_division) DO UPDATE SET pe_bank = EXCLUDED.pe_bank, pe_bank_acnt_name = EXCLUDED.pe_bank_acnt_name,
		   pe_bank_acnt_enc = EXCLUDED.pe_bank_acnt_enc, pe_level = EXCLUDED.pe_level, pe_cost_group = EXCLUDED.pe_cost_group`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	t.Cleanup(func() { // other report tests expect these NIKs without an employee row
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik IN ('ZREP0001','ZREP0002')`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: bankYYMM, Division: "SOLO"}
	if err := svc.ExportBank(ctx, q, "", report.MandiriBank, func(report.BankData) (int, error) { return 0, nil }); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("no key: %v", err)
	}
	td := time.Date(2036, 12, 26, 0, 0, 0, 0, time.UTC)
	txt := report.BankText{CompanyName: "PT. X", Branch: "SOLO", EscrowAccount: "9999", Signer1: "S1", Title1: "T1", Signer2: "S2", Title2: "T2",
		FeePerTransfer: decimal.NewFromInt(2200), TransferDate: td, PeriodDate: td}
	var buf bytes.Buffer
	var d report.BankData
	err := svc.ExportBank(ctx, q, encKey, report.MandiriBank, func(b report.BankData) (int, error) { d = b; return report.WriteBank(b, txt, &buf) })
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 1 || len(d.Issues) != 1 || d.Issues[0].NIK != "ZREP0002" {
		t.Fatalf("rows=%d issues=%+v", len(d.Rows), d.Issues)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if fm, _ := f.GetCellFormula(report.SheetBank, "D8"); fm != "SUM(D7:D7)" { //nolint:errcheck // asserted
		t.Fatalf("sum formula %q", fm)
	}
	if v, _ := f.GetCellValue(report.SheetBank, "D7", excelize.Options{RawCellValue: true}); v != "900" { //nolint:errcheck // asserted
		t.Fatalf("D7 = %q", v)
	}
	// Σ DETAIL = Σ net_pay of the payable rows = cover total = Σ transfer.
	if got := scalar(t, `SELECT COALESCE(sum(psh_net_pay),0)::text FROM payroll.pyr_salary_head WHERE psh_yymm=`+itoa(bankYYMM)+` AND psh_nik='ZREP0001'`); got != "900.00" || !d.Total().Equal(decimal.NewFromInt(900)) {
		t.Fatalf("db sum %s total %s", got, d.Total())
	}
	var rb bytes.Buffer
	var rd report.RecoData
	err = svc.ExportReco(ctx, q, encKey, func(r report.RecoData) (int, error) { rd = r; return report.WriteReco(r, &rb) })
	if err != nil {
		t.Fatal(err)
	}
	sum := decimal.Zero
	for _, r := range rd.Transfer.Rows {
		sum = sum.Add(r.Net)
	}
	if !sum.Equal(d.Total()) {
		t.Fatalf("Σ transfer %s != Σ bank %s", sum, d.Total())
	}
	rf, err := excelize.OpenReader(&rb)
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := rf.GetRows(report.SheetTransfer); len(rows) != 5 || rows[3][2] != "NIK" { //nolint:errcheck // asserted
		t.Fatalf("transfer sheet %v", rows)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT' AND pal_after->>'jenis'='bank'
	  AND (pal_after->>'baris')::int = 1 AND (pal_after->>'pra_cek')::int = 1`); got != "1" {
		t.Fatalf("audit rows = %s", got)
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_after::text LIKE '%1234567890%' OR pal_after::text LIKE '%9876543210%' OR pal_after::text LIKE '%ZREP%'`); got != "0" {
		t.Fatal("audit holds PII")
	}
}
