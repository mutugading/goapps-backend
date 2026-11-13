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

func writeGl(t *testing.T, rows ...string) etl.PSVDir {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MGT_PAYROLL_SETUP.psv"), []byte(glHdr+strings.Join(rows, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return etl.PSVDir{Dir: dir}
}

func glRow(jab, no, jenis, dept, akun, main, sub, drcr, desc, typ, div string) string {
	return strings.Join([]string{jab, no, jenis, "u", dept, akun, main, sub, drcr, desc, typ, div}, "|")
}

// T028k (11§4.5): MGT_PAYROLL_SETUP -> pyr_gl_setup + pyr_gl_group_map; main-sub, duplikat, Employee Balance, idempoten.
func TestETL_GlSetup(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	dir := writeGl(t,
		glRow("Operator", "1", "Tunjangan", "MGT", "405300-405301", "", "", "D", "Gaji Pokok", "GAJI", "SOLO"),  // main-sub string
		glRow("Operator", "32", "Potongan", "MB", "", "210100", "210101", "C", "Pinjaman", "GAJI", "SOLO"),      // kolom MAIN/SUB
		glRow("Operator", "0", "Employee Balance", "MGT", "", "220000", "", "C", "Hutang Gaji", "GAJI", "SOLO"), // Employee Balance
		glRow("Operator", "1", "Tunjangan", "MGT", "405300-405301", "", "", "D", "Gaji Pokok", "GAJI", "SOLO"),  // duplikat identik
		glRow("Executive", "1", "Tunjangan", "", "500000-500001", "", "", "D", "Gaji Exec", "THR", "SOLO"),      // dept kosong -> NULL
		glRow("Executive", "1", "Tunjangan", "", "500000-500009", "", "", "D", "Gaji Exec", "THR", "SOLO"),      // duplikat beda akun
		glRow("Operator", "999", "Tunjangan", "MGT", "1-2", "", "", "D", "Tanpa peta", "GAJI", "SOLO"),          // tanpa peta sysid
		glRow("Operator", "1", "Tunjangan", "MGT", "", "", "", "D", "Tanpa akun", "GAJI", "JAKARTA"),            // akun tak terurai
		glRow("Operator", "1", "Tunjangan", "XX", "1-2", "", "", "D", "Dept aneh", "GAJI", "SOLO"),              // dept tak dikenal
		glRow("Operator", "1", "Tunjangan", "MGT", "1-2", "", "", "D", "Tipe aneh", "XYZ", "SOLO"),              // MPS_TYPE salah
	)
	o := etl.GlSetupOptions{CompanyID: 1, ExpectRows: 10}
	state := func() string {
		return scalar(t, `SELECT md5(string_agg(x, ',' ORDER BY x)) FROM (
		  SELECT 's'||pgs_division||pgs_journal_type||pgs_gl_group||coalesce(pgs_cost_group,'-')||coalesce(pgs_component_sysid::text,'-')||
		    pgs_line_kind||pgs_account_main||coalesce(pgs_account_sub,'-')||pgs_sub_from_employee||pgs_drcr||pgs_description||pgs_valid_from AS x FROM payroll.pyr_gl_setup
		  UNION ALL SELECT 'g'||pggm_division||pggm_level||pggm_gl_group FROM payroll.pyr_gl_group_map) q`)
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
	// Dimuat: 3 unik + 1 duplikat identik (digabung) = 4 baris sumber; 6 exception (2 duplikat beda akun + 4 tak terurai).
	if rep.SrcRows != 10 || rep.DstRows != 4 || len(rep.Exceptions) != 6 {
		t.Fatalf("src=%d dst=%d exc=%d, mau 10/4/6: %v", rep.SrcRows, rep.DstRows, len(rep.Exceptions), rep.Exceptions)
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_gl_setup`: "3",
		`SELECT pgs_account_main||'/'||pgs_account_sub FROM payroll.pyr_gl_setup WHERE pgs_cost_group='MGT' AND pgs_line_kind='COMPONENT'`: "405300/405301",
		`SELECT pgs_account_main||'/'||pgs_account_sub FROM payroll.pyr_gl_setup WHERE pgs_cost_group='MB'`:                                "210100/210101",
		`SELECT pgs_sub_from_employee::text||pgs_drcr FROM payroll.pyr_gl_setup WHERE pgs_line_kind='EMPLOYEE_BALANCE'`:                    "trueC",
		`SELECT pgs_legacy_row->>'MPS_DESCRIPTION' FROM payroll.pyr_gl_setup WHERE pgs_line_kind='EMPLOYEE_BALANCE'`:                       "Hutang Gaji",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='JAKARTA' AND pggm_level='Office Boy'`:                     "Staff HO",
		`SELECT pggm_gl_group FROM payroll.pyr_gl_group_map WHERE pggm_division='SOLO' AND pggm_level='Operator Shift I,II,III'`:           "Operator",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s\n  got %q, mau %q", q, got, want)
		}
	}
}
