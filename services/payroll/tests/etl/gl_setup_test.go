package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

const glHdr = "MPS_JABATAN|MPS_NO|MPS_JENIS|MPS_URAIAN|MPS_DEPT|MPS_AKUN|MPS_MAIN|MPS_SUB|MPS_DRCR|MPS_DESCRIPTION|MPS_TYPE|MPS_DIVISION\n"

func writePSV(t *testing.T, table, hdr string, rows ...string) etl.PSVDir {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, table+".psv"), []byte(hdr+strings.Join(rows, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return etl.PSVDir{Dir: dir}
}

func glRow(jab, no, jenis, dept, akun, main, sub, drcr, desc, typ, div string) string {
	return strings.Join([]string{jab, no, jenis, "u", dept, akun, main, sub, drcr, desc, typ, div}, "|")
}

// T028k + T028p (11§4.5, D-63): MGT_PAYROLL_SETUP -> pyr_gl_setup + pyr_gl_group_map; main-sub, duplikat, PAYABLE,
// Employee Balance sebagai flag sub karyawan (bukan baris jurnal), Driver HRMC -> Driver Contract (dept), idempoten.
func TestETL_GlSetup(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	dir := writePSV(t, "MGT_PAYROLL_SETUP", glHdr,
		glRow("Operator", "1", "Tunjangan", "MGT", "405300-405301", "", "", "D", "Gaji Pokok", "GAJI", "SOLO"),       // main-sub string
		glRow("Operator", "32", "Potongan", "MB", "", "210100", "210101", "C", "Pinjaman", "GAJI", "SOLO"),           // kolom MAIN/SUB
		glRow("Operator", "6", "Tunjangan", "MGT", "105013", "105013", "", "D", "OT In Work", "GAJI", "SOLO"),        // ber-sub karyawan
		glRow("", "", "Employee Balance", "", "105013", "105013", "", "", "", "GAJI", "SOLO"),                        // flag akun 105013
		glRow("", "", "Employee Balance", "", "105099", "105099", "", "", "", "GAJI", "SOLO"),                        // flag tanpa komponen
		glRow("Operator", "", "Salary Payables", "", "205012", "205012", "", "C", "Salary Payables", "GAJI", "SOLO"), // PAYABLE
		glRow("Driver Contract", "", "Salary Payables", "", "205000", "205000", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("Operator", "", "THR Payables", "", "205000", "205000", "", "C", "THR Payables", "THR", "SOLO"),  // PAYABLE THR
		glRow("", "", "Salary Payables", "", "205000", "205000", "", "C", "Tanpa jabatan", "GAJI", "SOLO"),     // payable tanpa jabatan
		glRow("Operator", "1", "Tunjangan", "MGT", "405300-405301", "", "", "D", "Gaji Pokok", "GAJI", "SOLO"), // duplikat identik
		glRow("Executive", "1", "Tunjangan", "", "500000-500001", "", "", "D", "Gaji Exec", "THR", "SOLO"),     // dept kosong -> NULL
		glRow("Executive", "1", "Tunjangan", "", "500000-500009", "", "", "D", "Gaji Exec", "THR", "SOLO"),     // duplikat beda akun
		glRow("Operator", "999", "Tunjangan", "MGT", "1-2", "", "", "D", "Tanpa peta", "GAJI", "SOLO"),         // tanpa peta sysid
		glRow("Operator", "1", "Tunjangan", "MGT", "", "", "", "D", "Tanpa akun", "GAJI", "JAKARTA"),           // akun tak terurai
		glRow("Operator", "1", "Tunjangan", "XX", "1-2", "", "", "D", "Dept aneh", "GAJI", "SOLO"),             // dept tak dikenal
		glRow("Operator", "1", "Tunjangan", "MGT", "1-2", "", "", "D", "Tipe aneh", "XYZ", "SOLO"),             // MPS_TYPE salah
	)
	o := etl.GlSetupOptions{CompanyID: 1, ExpectRows: 16}
	state := func() string {
		return scalar(t, `SELECT md5(string_agg(x, ',' ORDER BY x)) FROM (
		  SELECT 's'||pgs_division||pgs_journal_type||pgs_gl_group||coalesce(pgs_cost_group,'-')||coalesce(pgs_component_sysid::text,'-')||
		    pgs_line_kind||pgs_account_main||coalesce(pgs_account_sub,'-')||pgs_sub_from_employee||pgs_drcr||pgs_description||pgs_valid_from AS x FROM payroll.pyr_gl_setup
		  UNION ALL SELECT 'g'||pggm_division||pggm_level||coalesce(pggm_dept_code,'-')||pggm_gl_group FROM payroll.pyr_gl_group_map) q`)
	}
	rep, err := etl.RunGlSetup(ctx, etlDB, dir, o)
	if err != nil {
		t.Fatal(err)
	}
	first := state()
	if _, err = etl.RunGlSetup(ctx, etlDB, dir, o); err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	// Dimuat: 6 baris jurnal unik + 1 duplikat identik + 2 Employee Balance (flag) = 9 baris sumber;
	// 7 exception (payable tanpa jabatan + 2 duplikat beda akun + 4 tak terurai).
	if rep.SrcRows != 16 || rep.DstRows != 9 || len(rep.Exceptions) != 7 {
		t.Fatalf("src=%d dst=%d exc=%d, mau 16/9/7: %v", rep.SrcRows, rep.DstRows, len(rep.Exceptions), rep.Exceptions)
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), "akun 105099 tanpa baris komponen") {
		t.Errorf("warning Employee Balance tanpa komponen tidak ada: %v", rep.Warnings)
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_gl_setup`:                                                                                        "6",
		`SELECT count(*) FROM payroll.pyr_gl_setup WHERE pgs_line_kind='EMPLOYEE_BALANCE'`:                                                 "0",
		`SELECT pgs_account_main||'/'||pgs_account_sub FROM payroll.pyr_gl_setup WHERE pgs_cost_group='MGT' AND pgs_account_main='405300'`: "405300/405301",
		`SELECT pgs_account_main||'/'||pgs_account_sub FROM payroll.pyr_gl_setup WHERE pgs_cost_group='MB'`:                                "210100/210101",
		`SELECT string_agg(pgs_account_main||':'||pgs_sub_from_employee, ',' ORDER BY pgs_account_main) FROM payroll.pyr_gl_setup
		  WHERE pgs_line_kind='COMPONENT' AND pgs_journal_type='GAJI'`: "105013:true,210100:false,405300:false",
		`SELECT string_agg(pgs_journal_type||'/'||pgs_gl_group||'/'||pgs_account_main||'/'||pgs_drcr||'/'||coalesce(pgs_component_sysid::text,'-')||'/'||pgs_sub_from_employee,
		  ',' ORDER BY pgs_journal_type, pgs_gl_group) FROM payroll.pyr_gl_setup WHERE pgs_line_kind='PAYABLE'`: "GAJI/Driver Contract/205000/C/-/false,GAJI/Operator/205012/C/-/false,THR/Operator/205000/C/-/false",
		`SELECT pgs_legacy_row->>'MPS_JENIS' FROM payroll.pyr_gl_setup WHERE pgs_line_kind='PAYABLE' AND pgs_journal_type='THR'`:           "THR Payables",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='JAKARTA' AND pggm_level='Office Boy'`:                     "Staff HO",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='SOLO' AND pggm_level='Operator Shift I,II,III'`:           "Operator",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='SOLO' AND pggm_level='Driver' AND pggm_dept_code IS NULL`: "Operator",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='SOLO' AND pggm_level='Driver' AND pggm_dept_code='HRMC'`:  "Driver Contract",
		// Resolusi prioritas dept (pola LATERAL reco/bank): HRMC -> Driver Contract, dept lain -> Operator.
		`SELECT string_agg(d||'='||(SELECT x.pggm_gl_group FROM payroll.pyr_gl_group_map x WHERE x.pggm_company_id=1 AND x.pggm_division='SOLO'
		  AND x.pggm_level='Driver' AND (x.pggm_dept_code IS NULL OR x.pggm_dept_code=d) ORDER BY (x.pggm_dept_code IS NULL) LIMIT 1), ',' ORDER BY d)
		  FROM unnest(ARRAY['HRMC','PRD']) d`: "HRMC=Driver Contract,PRD=Operator",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s\n  got %q, mau %q", q, got, want)
		}
	}
}

const fxHdr = "CER_EFF_FRM_DT|CER_EFF_TO_DT|CER_CONV_FM_CURR_CODE|CER_CONV_TO_CURR_CODE|CER_EXG_RATE_TYPE|CER_EXG_RATE|CER_UPD_UID\n"

// T028p (D-63 kurs): FM_EXCHANGE_RATE IDR→USD BOOK -> pyr_fx_rate; 12 desimal utuh, filter tipe/mata uang, idempoten.
func TestETL_FxRate(t *testing.T) {
	ctx := context.Background()
	dir := writePSV(t, "FM_EXCHANGE_RATE", fxHdr,
		"2026-10-01 00:00:00|2026-10-31 00:00:00|IDR|USD|B|.000055561729|FIN9", // 202610 contoh upload (12 desimal)
		"2026-09-01 00:00:00|2026-09-30 00:00:00|IDR|USD|B|.0000564876|FIN9",
		"2012-01-02 00:00:00|2012-01-08 00:00:00|IDR|USD|B|.00011|FIN9", // dua kurs dalam satu bulan -> warning
		"2012-01-09 00:00:00|2012-01-15 00:00:00|IDR|USD|B|.00012|FIN9",
		"2026-10-01 00:00:00|2026-10-31 00:00:00|IDR|USD|T|.00006|FIN9",         // tipe TAX: tidak dimuat
		"2026-10-01 00:00:00|2026-10-31 00:00:00|USD|IDR|B|16000|FIN9",          // arah lain: tidak dimuat
		"2026-08-01 00:00:00|2026-08-31 00:00:00|IDR|USD|B|0|FIN9",              // kurs 0 -> exception
		"bukan-tanggal|2026-08-31 00:00:00|IDR|USD|B|.00005|FIN9",               // tanggal rusak -> exception
		"2026-07-01 00:00:00|2026-07-31 00:00:00|IDR|USD|B|.1234567890123|FIN9", // 13 desimal -> exception (tidak dipotong)
	)
	state := func() string {
		return scalar(t, `SELECT coalesce(string_agg(pfr_rate_date||pfr_from_ccy||pfr_to_ccy||pfr_rate_type||pfr_rate||pfr_source, ',' ORDER BY pfr_rate_date),'')
		  FROM payroll.pyr_fx_rate`)
	}
	rep, err := etl.RunFxRate(ctx, etlDB, dir, etl.FxRateOptions{ExpectRows: 7})
	if err != nil {
		t.Fatal(err)
	}
	first := state()
	if _, err = etl.RunFxRate(ctx, etlDB, dir, etl.FxRateOptions{}); err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	if rep.SrcRows != 9 || rep.DstRows != 4 || len(rep.Exceptions) != 3 || len(rep.Warnings) != 1 {
		t.Fatalf("src=%d dst=%d exc=%d warn=%d, mau 9/4/3/1: %v %v", rep.SrcRows, rep.DstRows, len(rep.Exceptions), len(rep.Warnings), rep.Exceptions, rep.Warnings)
	}
	if got := scalar(t, `SELECT pfr_rate::text FROM payroll.pyr_fx_rate WHERE pfr_rate_date='2026-10-01' AND pfr_rate_type='BOOK'`); got != "0.000055561729" {
		t.Errorf("kurs 202610 = %s, mau 0.000055561729 (tanpa pemotongan)", got)
	}
	// LC = ROUND(FC × rate, 2) memakai kurs utuh (Σ D contoh 202610 SOLO = 4.009.009.997).
	if got := scalar(t, `SELECT round(4009009997 * pfr_rate, 2)::text FROM payroll.pyr_fx_rate WHERE pfr_rate_date='2026-10-01'`); got != "222747.53" {
		t.Errorf("LC contoh = %s", got)
	}
	if _, err := etl.RunFxRate(ctx, etlDB, dir, etl.FxRateOptions{ExpectRows: 99}); err == nil {
		t.Error("ExpectRows salah harus gagal")
	}
}
