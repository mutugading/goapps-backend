package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
)

// T028e/T028o: slip PDF (layout F-3) data path against PG18 as payroll_app: encrypted with DOB password, NULL DOB
// skipped, -nopassword, zero lines not printed, NPWP/account decrypted into the PDF only (never audit), Basis/Remarks,
// attendance box from the run snapshot, audit row without NIK. Three synthetic NIKs.
const slipYYMM = 203610

func TestReport_Slip(t *testing.T) {
	seedReportRun(t, slipYYMM, "APPROVED")
	for _, q := range []string{
		// the migration seed (000041) finds no MGT company at migrate time in this harness
		`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
		 VALUES (1, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'test', 'system') ON CONFLICT DO NOTHING`,
		`UPDATE payroll.pyr_salary_head SET psh_rate_perjam = 10000, psh_jam_lembur = '{"lembur1": 2.5}'::jsonb
		 WHERE psh_yymm = ` + itoa(slipYYMM),
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_birth_date, pe_npwp_enc, pe_bank_acnt_enc)
		 VALUES (1,'ZREP0001','SOLO','Rep Satu','ZREP0001','1990-03-07', payroll.pgp_sym_encrypt('99.111.222.3-444.000','` + encKey + `'), payroll.pgp_sym_encrypt('5550001111','` + encKey + `')),
		        (1,'ZREP0002','SOLO','Rep Dua','ZREP0002',NULL, NULL, NULL),
		        (1,'ZREP0003','SOLO','Rep Tiga','ZREP0003','1985-12-31', payroll.pgp_sym_encrypt('99.333.444.5-666.000','` + encKey + `'), payroll.pgp_sym_encrypt('5550003333','` + encKey + `'))
		 ON CONFLICT DO NOTHING`,
		// other report tests seed the same NIKs without NPWP: set the encrypted values explicitly
		`UPDATE payroll.pyr_employee SET pe_npwp_enc = payroll.pgp_sym_encrypt('99.111.222.3-444.000','` + encKey + `'),
		   pe_bank_acnt_enc = payroll.pgp_sym_encrypt('5550001111','` + encKey + `') WHERE pe_nik = 'ZREP0001'`,
		`UPDATE payroll.pyr_employee SET pe_npwp_enc = NULL, pe_bank_acnt_enc = NULL WHERE pe_nik = 'ZREP0002'`,
		`UPDATE payroll.pyr_salary_head SET psh_dept_code='D01', psh_section='Rep Section', psh_grade='G5', psh_tax_code='K1', psh_pay_date='2036-01-26'
		 WHERE psh_yymm = ` + itoa(slipYYMM),
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division, psh_nama, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT psh_run_id,1,'ZREP0003',psh_yymm,'SOLO','Rep Tiga',700,0,0 FROM payroll.pyr_salary_head WHERE psh_nik='ZREP0001' AND psh_yymm = ` + itoa(slipYYMM),
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount, psd_basis_amount, psd_remarks)
		 SELECT psd_run_id,'ZREP0003','Tunjangan'::payroll.comp_type,901,700,0,'rem tiga' FROM payroll.pyr_salary_detail WHERE psd_nik='ZREP0001' AND psd_id=901
		   AND psd_run_id IN (SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm = ` + itoa(slipYYMM) + `)`,
		`UPDATE payroll.pyr_salary_detail SET psd_basis_amount = 123456, psd_remarks = 'Remarks sintetis baris satu
baris dua' WHERE psd_nik='ZREP0001' AND psd_id=901 AND psd_run_id IN (SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm = ` + itoa(slipYYMM) + `)`,
		// malformed recap values (empty string, text) must count as 0, not fail the batch
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT pas_snapshot_id,'REKAP','ZREP0003','{"MPAR_DAY_WORK": "", "MPAR_ALPA": "abc", "MPAR_IJIN": null, "MPAR_CUTI": 2}'::jsonb
		 FROM payroll.pyr_att_snapshot WHERE pas_checksum='rep-` + itoa(slipYYMM) + `'`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT pas_snapshot_id,'REKAP','ZREP0001','{"MPAR_DAY_WORK": 21, "MPAR_DAYOFF": 4, "MPAR_CUTI": 1, "MPAR_ALPA": 0, "MPAR_IJIN": 2}'::jsonb
		 FROM payroll.pyr_att_snapshot WHERE pas_checksum='rep-` + itoa(slipYYMM) + `'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	svc := report.NewService(appDB, auditinfra.NewWriter())
	ctx := ctxFor(t, userRW)
	q := report.Query{CompanyID: 1, YYMM: slipYYMM, Division: "SOLO"}
	cfg := report.SlipConfig{PIIKey: encKey, ContactIT: "it-sintetis@example.test"}

	if _, err := svc.ExportSlips(ctx, q, report.SlipConfig{}, report.SlipOptions{OutDir: t.TempDir(), PeriodName: "x"}); !errors.Is(err, report.ErrNoPIIKey) {
		t.Fatalf("tanpa kunci PII harus ditolak: %v", err)
	}

	dir := t.TempDir()
	res, err := svc.ExportSlips(ctx, q, cfg, report.SlipOptions{OutDir: dir, PeriodName: "Februari 2036"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 2 || res.Written[0] != "ZREP0001" || res.Written[1] != "ZREP0003" || len(res.Skipped) != 1 || res.Skipped[0] != "ZREP0002" {
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
	  AND pal_after->>'jenis'='slip' AND (pal_after->>'baris')::int=2 AND (pal_after->>'dilewati')::int=1 AND pal_after::text NOT LIKE '%ZREP%'`); got != "1" {
		t.Fatalf("audit rows = %s", got)
	}
	// NPWP and account number never reach audit or any log-like table
	if got := scalar(t, `SELECT count(*)::text FROM payroll.pyr_audit_log WHERE pal_after::text LIKE '%5550001111%' OR pal_after::text LIKE '%99.111%'
	  OR pal_before::text LIKE '%5550001111%' OR pal_before::text LIKE '%99.111%'`); got != "0" {
		t.Fatalf("PII di audit: %s", got)
	}

	// finance print: no password, all NIKs, readable content with no zero lines
	dir2 := t.TempDir()
	res, err = svc.ExportSlips(ctx, q, cfg, report.SlipOptions{OutDir: dir2, NoPassword: true, NIK: "ZREP0002", PeriodName: "Februari 2036"})
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
	// NIK 0002 has no NPWP/account/recap: those cells stay empty, the slip still renders
	if contains(plain, "5550001111") {
		t.Fatal("rekening NIK lain bocor")
	}

	// F-3 content of NIK 0001 (decrypted with its password; PDF content is plain once the file is decrypted)
	dir3 := t.TempDir()
	if _, err := svc.ExportSlips(ctx, q, cfg, report.SlipOptions{OutDir: dir3, NoPassword: true, NIK: "ZREP0001", PeriodName: "Februari 2036"}); err != nil {
		t.Fatal(err)
	}
	one, err := os.ReadFile(filepath.Join(dir3, "203610GZREP0001.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"99.111.222.3-444.000", "5550001111", "D01 Rep Section", "G5", "K1", "26-JAN-36", "123.456", "Remarks sintetis",
		"baris dua", "it-sintetis@example.test", "Informasi", "Net Paid"} {
		if !contains(one, want) {
			t.Errorf("slip NIK 0001 tidak memuat %q", want)
		}
	}
	if !contains(enc, "/Encrypt") || contains(enc, "5550001111") {
		t.Fatal("PDF berpassword memuat rekening sebagai teks biasa")
	}

	// -nik decrypts only that NIK: a corrupt ciphertext of another NIK must not matter
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_npwp_enc = '\x00'::bytea WHERE pe_nik = 'ZREP0003'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportSlips(ctx, q, cfg, report.SlipOptions{OutDir: t.TempDir(), NoPassword: true, NIK: "ZREP0001", PeriodName: "x"}); err != nil {
		t.Fatalf("-nik harus hanya mendekripsi NIK itu: %v", err)
	}
	if _, err := svc.ExportSlips(ctx, q, cfg, report.SlipOptions{OutDir: t.TempDir(), NoPassword: true, PeriodName: "x"}); err == nil {
		t.Fatal("tanpa -nik, ciphertext rusak harus menggagalkan batch")
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
