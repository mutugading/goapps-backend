package etl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033h (review #283 #6): reconciliation counts the rows of this run, not the whole table; an employee created
// outside the ETL does not fail later loads; a placeholder/invalid NPWP is stored NULL (no hash) with a warning.
func TestT033h_EmployeeReconPerRunAndNPWP(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	// Employee + attachment that exist only in the app (not in the spool).
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_division, pe_nik, pe_nama, pe_sts_payroll, pe_orion_sub_acnt, created_by)
		   VALUES (1,'SOLO','T33H-APP','Dari App',2,'T33H-APP','app') ON CONFLICT DO NOTHING`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik IN ('T33H-APP')`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	o := etl.EmployeeOptions{CompanyID: 1, EncKey: "test-enc-key-0123456789abcdef0123", HMACKey: "test-hmac-key-0123456789abcdef012"}
	reps, err := etl.RunEmployees(ctx, etlDB, src, o)
	if err != nil {
		t.Fatalf("karyawan dari app menggagalkan ETL: %v", err)
	}
	for _, r := range reps {
		if !r.OK() {
			t.Errorf("%s src=%d dst=%d exc=%d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
	}

	// NPWP placeholder / wrong length: NULL + warning, never a shared hash.
	dir := t.TempDir()
	b, err := os.ReadFile(filepath.Join("testdata", "MGT_PAYROLL_EMP.psv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	hdr := strings.Split(lines[0], "|")
	col := map[string]int{}
	for i, h := range hdr {
		col[h] = i
	}
	row := func(nik, npwp string) string {
		r := make([]string, len(hdr))
		r[col["PE_NIK"]], r[col["PE_NAMA"]], r[col["PE_DIVISION"]], r[col["PE_NPWP"]], r[col["PE_STS_PAYROLL"]] = nik, "N "+nik, "SOLO", npwp, "2"
		return strings.Join(r, "|")
	}
	emp := strings.Join([]string{lines[0], row("T33H-Z1", "00.000.000.0-000.000"), row("T33H-Z2", "00.000.000.0-000.000"),
		row("T33H-S", "12.345.678"), row("T33H-OK", "11.222.333.4-555.000")}, "\n") + "\n"
	ce := "MPCE_NIK|MPCE_TYPE|MPCE_ID|MPCE_NAME|MPCE_STS|MPCE_TAX|MPCE_AMOUNT|MPCE_FRZ_FLAG|MPCE_LEVEL|MPCE_TAX_PAID_COMPANY|MPCE_DIVISION\n"
	for f, c := range map[string]string{"MGT_PAYROLL_EMP.psv": emp, "MGT_PAYROLL_COMPNT_EMP.psv": ce} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee_nik_hist WHERE penh_nik LIKE 'T33H-%'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik LIKE 'T33H-%'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	reps, err = etl.RunEmployees(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.EmployeeOptions{CompanyID: 1, EncKey: o.EncKey, HMACKey: o.HMACKey, Divisions: []string{"SOLO"}})
	if err != nil {
		t.Fatal(err)
	}
	if reps[0].DstRows != 4 || !reps[0].OK() {
		t.Errorf("rekonsiliasi per run: src=%d dst=%d", reps[0].SrcRows, reps[0].DstRows)
	}
	w := strings.Join(reps[0].Warnings, "\n")
	if strings.Count(w, "NPWP") != 3 || strings.Contains(w, "000.000") || strings.Contains(w, "12.345.678") {
		t.Errorf("warning NPWP (3, tanpa nilai PII): %q", w)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_employee WHERE pe_nik IN ('T33H-Z1','T33H-Z2','T33H-S') AND pe_npwp_enc IS NULL AND pe_npwp_hash IS NULL`); got != "3" {
		t.Errorf("NPWP tidak valid tersimpan: %s/3 NULL", got)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_employee WHERE pe_nik='T33H-OK' AND pe_npwp_hash IS NOT NULL`); got != "1" {
		t.Errorf("NPWP valid tidak di-hash")
	}
}

// T033h: att-legacy-fail only updates the snapshot of its company (NIK + period may repeat in another company).
func TestT033h_AttLegacyFailCompanyFilter(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.RunAttLegacy(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	var other int
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('T3H','T033h') ON CONFLICT (pco_code) DO UPDATE SET pco_name=EXCLUDED.pco_name RETURNING pco_company_id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(fmt.Sprintf(`INSERT INTO payroll.pyr_division_ref VALUES (%d,'SOLO','Solo') ON CONFLICT DO NOTHING`, other)); err != nil {
		t.Fatal(err)
	}
	var snap int64
	if err := ownDB.QueryRow(fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		VALUES (%d,202606,'SOLO','{}','t033h',false) RETURNING pas_snapshot_id`, other)).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{fmt.Sprintf(`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d`, snap),
			fmt.Sprintf(`DELETE FROM payroll.pyr_att_snapshot WHERE pas_snapshot_id=%d`, snap)} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
	if _, err := ownDB.Exec(fmt.Sprintf(`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
		VALUES (%d,'REKAP','2026-0001','{}',7,7)`, snap)); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, etl.PSVDir{Dir: "testdata_fail"}, etl.AttLegacyFailOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT pasr_premi_fail||'/'||pasr_beras_fail FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id=%d`, snap)); got != "7/7" {
		t.Errorf("snapshot company lain ikut diubah: %s", got)
	}
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, etl.PSVDir{Dir: "testdata_fail"}, etl.AttLegacyFailOptions{}); err == nil {
		t.Error("tanpa CompanyID diterima")
	}
}
