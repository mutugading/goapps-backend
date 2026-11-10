package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

const thrMainHdr = "TM_YYMM|TM_TYPE|TM_DESCRIPTION|TM_DATE|TM_NIK|TM_NAMA|TM_DEPT|TM_GRADE|TM_EMP_TYPE|TM_GAPOK|TM_DATE_JOIN|TM_TAX_CODE|TM_TOT_TUNJANGAN|TM_TOT_POTONGAN|TM_ROUNDING|TM_TOT_THR|TM_BANK_ACNT|TM_NPWP|TM_DEPT_CODE|TM_SECTION|TM_DIVISION\n"
const thrDetHdr = "TD_YYMM|TD_TYPE|TD_DESCRIPTION|TD_NIK|TD_NAMA|TD_DEPT|TD_EMP_TYPE|TD_NO|TD_JENIS|TD_URAIAN|TD_AMOUNT|TD_DIVISION\n"

func writeThr(t *testing.T, main, det string) etl.PSVDir {
	t.Helper()
	dir := t.TempDir()
	for f, c := range map[string]string{"MGT_THR_MAIN.psv": thrMainHdr + main, "MGT_THR_DETAIL.psv": thrDetHdr + det} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return etl.PSVDir{Dir: dir}
}

func thrMain(yymm, typ, desc, date, nik, tun, pot, rnd, div string) string {
	return strings.Join([]string{yymm, typ, desc, date, nik, "N", "D", "", "Op", "5000000", "01-JAN-20", "K1", tun, pot, rnd, "", "", "", "", "", div}, "|") + "\n"
}

func thrDet(yymm, typ, nik, no, jenis, amt, div string) string {
	return strings.Join([]string{yymm, typ, "d", nik, "N", "D", "Op", no, jenis, "uraian", amt, div}, "|") + "\n"
}

// T024b (D-56): MGT_THR_MAIN/DETAIL -> event CLOSED + run LEGACY ACTIVE + head/detail; idempotent; exceptions held per event.
func TestETL_ThrLegacy(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	thrEmployees(t)
	main := thrMain("202603", "THR", "THR 2026", "10-MAR-26", "1001", "350", "10", "40", "SOLO") +
		thrMain("202603", "THR", "THR 2026", "10-MAR-26", "1002", "200", "0", "0", "SOLO") +
		thrMain("202603", "THR", "THR JKT", "12-MAR-26", "2001", "100.005", "0", "0.005", "JAKARTA")
	det := thrDet("202603", "THR", "1001", "1", "Tunjangan", "300", "SOLO") +
		thrDet("202603", "THR", "1001", "800", "Tunjangan", "50", "SOLO") + // T800: tanpa peta (D-56)
		thrDet("202603", "THR", "1001", "32", "Potongan", "10", "SOLO") +
		thrDet("202603", "THR", "1002", "1", "Tunjangan", "200", "SOLO") +
		thrDet("202603", "THR", "2001", "1", "Tunjangan", "100.005", "JAKARTA")
	dir := writeThr(t, main, det)
	o := etl.ThrLegacyOptions{CompanyID: 1, ExpectMain: 3, ExpectDetail: 5}
	state := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(pte_division||pte_kind||pte_yymm||pte_status||pte_description||pte_event_date, ',' ORDER BY 1) FROM payroll.pyr_thr_event) ||
		  (SELECT string_agg(ptr_event_id||ptr_kind||ptr_status||ptr_emp_count||ptr_tot_tunjangan, ',' ORDER BY 1) FROM payroll.pyr_thr_run) ||
		  (SELECT string_agg(pth_nik||pth_net_pay, ',' ORDER BY 1) FROM payroll.pyr_thr_head) ||
		  (SELECT string_agg(ptd_nik||ptd_id||ptd_type||ptd_amount||coalesce(ptd_component_sysid::text,'-'), ',' ORDER BY 1) FROM payroll.pyr_thr_detail))`)
	}
	reps, err := etl.RunThrLegacy(ctx, etlDB, dir, o)
	if err != nil {
		t.Fatal(err)
	}
	first := state()
	reps2, err := etl.RunThrLegacy(ctx, etlDB, dir, o)
	if err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for i, r := range reps {
		if !r.OK() || !reps2[i].OK() || len(r.Exceptions) != 0 || r.DstRows != r.SrcRows {
			t.Errorf("%s src=%d dst=%d exc=%d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_thr_event WHERE pte_status='CLOSED' AND pte_decision='GIVEN' AND pte_active_run_id IS NOT NULL`: "2",
		`SELECT count(*) FROM payroll.pyr_thr_run WHERE ptr_kind='LEGACY' AND ptr_status='ACTIVE'`:                                        "2",
		`SELECT count(*) FROM payroll.pyr_thr_head`:   "3",
		`SELECT count(*) FROM payroll.pyr_thr_detail`: "5",
		`SELECT count(*) FROM payroll.pyr_thr_detail WHERE ptd_component_sysid IS NULL AND ptd_meta->>'legacy_unmapped'='true'`:                         "1",
		`SELECT pth_net_pay::text FROM payroll.pyr_thr_head WHERE pth_nik='1001'`:                                                                       "300.00",
		`SELECT pth_rounding::text FROM payroll.pyr_thr_head WHERE pth_nik='2001'`:                                                                      "0.01",
		`SELECT pte_description FROM payroll.pyr_thr_event WHERE pte_division='JAKARTA'`:                                                                "THR JKT",
		`SELECT sum(td_amount)::text FROM payroll.v_pyr_thr_detail WHERE td_jenis='Tunjangan' AND td_division='SOLO'`:                                   "550.00",
		`SELECT sum(ptd_amount)::text FROM payroll.pyr_thr_detail WHERE ptd_type='Potongan' AND NOT ptd_tax`:                                            "10.00",
		`SELECT ptr_tot_tunjangan::text FROM payroll.pyr_thr_run r JOIN payroll.pyr_thr_event e ON e.pte_id=r.ptr_event_id WHERE e.pte_division='SOLO'`: "550.00",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// event CLOSED menolak mutasi, juga dari pemilik skema.
	for _, q := range []string{
		`UPDATE payroll.pyr_thr_head SET pth_rounding = 1 WHERE pth_nik='1001'`,
		`DELETE FROM payroll.pyr_thr_detail WHERE ptd_nik='1001'`,
	} {
		if _, err := ownDB.Exec(q); err == nil {
			t.Errorf("event CLOSED menerima mutasi: %s", q)
		}
	}
	// sumber berubah setelah event CLOSED: ditolak, bukan ditimpa.
	changed := writeThr(t, main, det+thrDet("202603", "THR", "1002", "6", "Tunjangan", "1", "SOLO"))
	if _, err := etl.RunThrLegacy(ctx, etlDB, changed, etl.ThrLegacyOptions{CompanyID: 1}); err == nil {
		t.Error("sumber berbeda dari yang sudah dimuat diterima")
	}
	if state() != first {
		t.Fatal("penolakan mengubah data")
	}
}

// T024b: NIK tanpa pyr_employee dan DETAIL tanpa MAIN jadi exception; event ditahan utuh (tidak setengah dimuat).
func TestETL_ThrLegacyExceptions(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	thrEmployees(t)
	main := thrMain("202504", "Bonus", "Bonus 2025", "10-APR-25", "1001", "10", "0", "0", "SOLO") +
		thrMain("202504", "Bonus", "Bonus 2025", "10-APR-25", "7777", "10", "0", "0", "SOLO") + // NIK tak dikenal
		thrMain("202404", "Bonus", "Bonus 2024", "10-APR-24", "1002", "5", "0", "0", "SOLO") + // event sehat
		thrMain("202404", "THR", "x", "x", "1002", "5", "0", "0", "SOLO") // tanggal rusak: event ditahan
	det := thrDet("202504", "Bonus", "1001", "1", "Tunjangan", "10", "SOLO") +
		thrDet("202504", "Bonus", "7777", "1", "Tunjangan", "10", "SOLO") +
		thrDet("202404", "Bonus", "1002", "1", "Tunjangan", "5", "SOLO") +
		thrDet("202404", "Bonus", "1001", "1", "Tunjangan", "3", "SOLO") + // DETAIL tanpa MAIN -> event 202404 ditahan
		thrDet("202404", "THR", "1002", "1", "Tunjangan", "5", "SOLO") +
		thrDet("202404", "Lain", "1002", "1", "Tunjangan", "5", "SOLO") // jenis event tak dikenal
	reps, err := etl.RunThrLegacy(ctx, etlDB, writeThr(t, main, det), etl.ThrLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		if !r.OK() || r.DstRows != 0 {
			t.Errorf("%s src=%d dst=%d exc=%d, want semua exception", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_thr_event WHERE pte_yymm IN (202504, 202404)`); got != "0" {
		t.Errorf("event ditahan tetap termuat: %s", got)
	}
}

// thrEmployees makes sure NIK 1001/1002 (SOLO) and 2001 (JAKARTA) exist, whichever tests ran before (shared DB).
func thrEmployees(t *testing.T) {
	t.Helper()
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt)
SELECT 1, v.nik, v.div::payroll.division, 'T ' || v.nik, v.nik FROM (VALUES ('1001','SOLO'),('1002','SOLO'),('2001','JAKARTA')) v(nik, div)
WHERE NOT EXISTS (SELECT 1 FROM payroll.pyr_employee e WHERE e.pe_company_id=1 AND e.pe_nik=v.nik AND e.pe_division=v.div::payroll.division)`); err != nil {
		t.Fatal(err)
	}
}
