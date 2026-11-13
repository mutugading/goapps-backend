package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
)

// T028e: slip PDF data path against PG18 as payroll_app: encrypted with DOB password, NULL DOB skipped, -nopassword,
// zero lines not printed, audit row without NIK.
const slipYYMM = 203610

func TestReport_Slip(t *testing.T) {
	seedReportRun(t, slipYYMM, "APPROVED")
	for _, q := range []string{
		// the migration seed (000041) finds no MGT company at migrate time in this harness
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`UPDATE payroll.pyr_salary_head SET psh_rate_perjam = 10000, psh_jam_lembur = '{"lembur1": 2.5}'::jsonb
		 WHERE psh_yymm = ` + itoa(slipYYMM),
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07'), (1,'ZREP0002','SOLO','Rep Dua','ZREP0002',NULL)
		 ON CONFLICT DO NOTHING`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: slipYYMM, Division: "SOLO"}

	dir := t.TempDir()
	res, err := svc.ExportSlips(ctx, q, report.SlipOptions{OutDir: dir, PeriodName: "Februari 2036"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 || res.Written[0] != "ZREP0001" || len(res.Skipped) != 1 || res.Skipped[0] != "ZREP0002" {
		t.Fatalf("result %+v", res)
	}
	enc, err := os.ReadFile(filepath.Join(dir, "203610GZREP0001.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pdf.Validate(enc, "07031990"); err != nil {
		t.Fatalf("password benar: %v", err)
	}
	if err := pdf.Validate(enc, "00000000"); err == nil {
		t.Fatal("password salah lolos")
	}
	if _, err := os.Stat(filepath.Join(dir, "203610GZREP0002.pdf")); !os.IsNotExist(err) {
		t.Fatal("NIK tanpa tanggal lahir tidak boleh punya berkas")
	}
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_action='REPORT_EXPORT'
	  AND pal_after->>'jenis'='slip' AND (pal_after->>'baris')::int=1 AND (pal_after->>'dilewati')::int=1 AND pal_after::text NOT LIKE '%ZREP%'`); got != "1" {
		t.Fatalf("audit rows = %s", got)
	}

	// finance print: no password, all NIKs, readable content with no zero lines
	dir2 := t.TempDir()
	res, err = svc.ExportSlips(ctx, q, report.SlipOptions{OutDir: dir2, NoPassword: true, NIK: "ZREP0002", PeriodName: "Februari 2036"})
	if err != nil || len(res.Written) != 1 {
		t.Fatalf("nopassword: %+v %v", res, err)
	}
	plain, err := os.ReadFile(filepath.Join(dir2, "203610GZREP0002.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pdf.Validate(plain, ""); err != nil {
		t.Fatalf("plain pdf: %v", err)
	}
	if contains(plain, "Rep BPJS") || !contains(plain, "Rep Gaji") {
		t.Fatal("baris 0 (Potongan 902 = 0) tercetak atau Tunjangan hilang")
	}
}

func contains(b []byte, s string) bool { return len(b) > 0 && indexOf(b, s) >= 0 }

func indexOf(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
