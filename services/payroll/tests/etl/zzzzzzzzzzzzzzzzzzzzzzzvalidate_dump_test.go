package etl

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/recon"
)

// T032b: `recon validate -dump` compares the DB aggregates with the PSV dump. Range 201801..201802 is this test's own:
// SOLO loaded through the real loader (incl. an orphan NIK = D-35.2 synthetic MAIN), JAKARTA is D-35.1 skipped.
func TestValidateDump_CompareAndDetectDiff(t *testing.T) {
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := ownDB.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	exec(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
	 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001),(1,'P001','Potongan','BPJS','ACTIVE',202001)
	 ON CONFLICT (pc_company_id, pc_code) DO NOTHING`)
	exec(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
	 SELECT 1, c.pc_type, 1, 'SOLO', c.pc_sysid FROM payroll.pyr_component c WHERE c.pc_company_id = 1 AND c.pc_code IN ('T001','P001')
	 ON CONFLICT DO NOTHING`)

	dir := t.TempDir()
	mainCols := strings.Split("PM_YYMM|PM_NIK|PM_NAMA|PM_DEPT|PM_GRADE|PM_JABATAN|PM_EMP_TYPE|PM_DATE|PM_PEMBAYARAN|PM_DATE_JOIN|PM_BASIC_PAY|PM_TUNJJABATAN|PM_TUNJRUMAH|PM_TUNJKESEJAHTERAAN|PM_TUNJTRANSPORT|PM_RATE_PERJAM|PM_TAX_CODE|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING|PM_DAY_PAY|PM_JAM_LEMBUR1|PM_JAM_LEMBUR2|PM_JAM_LEMBURNAS1|PM_JAM_LEMBURNAS2|PM_JAM_LEMBURNAS3|PM_JAM_LEMBUROFF1|PM_JAM_LEMBUROFF2|PM_JAM_LEMBUROFF3|PM_JAM_LEMBURNASP1|PM_JAM_LEMBURNASP2|PM_JAM_LEMBURNASP3|PM_DEPT_CODE|PM_REMARKS|PM_SECTION|PM_DIVISION", "|")
	detCols := strings.Split("PD_YYMM|PD_NIK|PD_NO|PD_TYPE|PD_DESCRIPTION|PD_AMOUNT|PD_STATUS|PD_TAX|PD_BASIS_AMOUNT|PD_REMARKS|PD_TAX_PAID_COMPANY|PD_DIVISION", "|")
	row := func(cols []string, kv map[string]string) string {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = kv[c]
		}
		return strings.Join(cells, "|") + "\n"
	}
	mrow := func(yymm, nik, div, date, tun, pot, rnd string) string {
		return row(mainCols, map[string]string{"PM_YYMM": yymm, "PM_NIK": nik, "PM_DIVISION": div, "PM_DATE": date,
			"PM_TOT_TUNJANGAN": tun, "PM_TOT_POTONGAN": pot, "PM_ROUNDING": rnd})
	}
	drow := func(yymm, nik, typ, div, amt string) string {
		return row(detCols, map[string]string{"PD_YYMM": yymm, "PD_NIK": nik, "PD_TYPE": typ, "PD_NO": "1", "PD_DIVISION": div,
			"PD_AMOUNT": amt, "PD_DESCRIPTION": "x", "PD_STATUS": "F", "PD_TAX": "2", "PD_TAX_PAID_COMPANY": "N"})
	}
	mainPSV := strings.Join(mainCols, "|") + "\n" +
		mrow("201801", "W0001", "SOLO", "25-JAN-18", "1000.10", "200.05", "0.05") +
		mrow("201801", "W0002", "SOLO", "25-JAN-18", "300", "0", "0") +
		mrow("201802", "W0001", "SOLO", "25-FEB-18", "50.50", "0", "0") +
		mrow("201801", "W0003", "JAKARTA", "25-JAN-18", "77", "0", "0") // D-35.1: JAKARTA < 202601
	detPSV := strings.Join(detCols, "|") + "\n" +
		drow("201801", "W0001", "Tunjangan", "SOLO", "1000.10") +
		drow("201801", "W0001", "Potongan", "SOLO", "200.05") +
		drow("201801", "W0002", "Tunjangan", "SOLO", "300") +
		drow("201801", "W0009", "Tunjangan", "SOLO", "100.005") + // orphan: synthetic MAIN 100.01
		drow("201802", "W0001", "Tunjangan", "SOLO", "50.50") +
		drow("201801", "W0003", "Tunjangan", "JAKARTA", "77")
	for f, c := range map[string]string{"MGT_PAYROLL_MAIN.psv": mainPSV, "MGT_PAYROLL_DETAIL.psv": detPSV} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reps, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.SalaryLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		if !r.OK() || len(r.Exceptions) != 0 {
			t.Fatalf("%s src=%d dst=%d exc=%v", r.Table, r.SrcRows, r.DstRows, r.Exceptions)
		}
	}

	raw, err := sql.Open("pgx", strings.Replace(appDSN, "user=payroll_app password=payroll_app123", "user=payroll_recon password=payroll_recon123", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	q := recon.Query{CompanyID: 1, YYMMFrom: 201801, YYMMTo: 201802}
	run := func() recon.Report {
		t.Helper()
		rep, err := recon.Validate(ctx, raw, q)
		if err != nil {
			t.Fatal(err)
		}
		if err := recon.CompareDump(&rep, etl.PSVDir{Dir: dir}); err != nil {
			t.Fatal(err)
		}
		return rep
	}

	rep := run()
	c := rep.DumpCompare
	if c == nil || !c.OK() || c.MatchedPeriods != 2 || len(c.Diffs) != 0 {
		t.Fatalf("clean dump must match: %+v", c)
	}
	if len(c.SkippedD351) != 1 || c.SkippedD351[0].Division != "JAKARTA" || c.SkippedD351[0].MainRows != 1 || c.SkippedD351[0].DetailRows != 1 {
		t.Errorf("skip D-35.1 reported apart: %+v", c.SkippedD351)
	}
	if len(c.SyntheticD352) != 1 || c.SyntheticD352[0].YYMM != 201801 || c.SyntheticD352[0].Rows != 1 {
		t.Errorf("synthetic D-35.2: %+v", c.SyntheticD352)
	}
	if !rep.OK() && len(rep.Failures) != 0 && strings.Contains(strings.Join(rep.Failures, " "), "dump") {
		t.Errorf("clean compare must not fail on the dump: %v", rep.Failures)
	}

	// Change one nominal in the DB (guard trigger off, owner role): the diff must name period, division and figure.
	exec(`ALTER TABLE payroll.pyr_salary_detail DISABLE TRIGGER trg_guard_line`)
	defer func() { _, _ = ownDB.Exec(`ALTER TABLE payroll.pyr_salary_detail ENABLE TRIGGER trg_guard_line`) }() //nolint:errcheck // cleanup
	exec(`UPDATE payroll.pyr_salary_detail d SET psd_amount = psd_amount + 1 FROM payroll.pyr_run r
	 WHERE d.psd_run_id = r.pr_id AND r.pr_yymm = 201802 AND r.pr_division = 'SOLO' AND d.psd_type = 'Tunjangan'`)
	rep = run()
	c = rep.DumpCompare
	if rep.OK() || c.OK() || len(c.Diffs) != 1 {
		t.Fatalf("one changed nominal => exactly one diff, report not OK: ok=%v %+v", rep.OK(), c.Diffs)
	}
	df := c.Diffs[0]
	if df.YYMM != 201802 || df.Division != "SOLO" || df.Field != recon.FieldDetailT || df.DB != "51.50" || df.Dump != "50.50" || df.Delta != "-1.00" {
		t.Errorf("diff %+v", df)
	}
	if c.MatchedPeriods != 1 {
		t.Errorf("matched %d, want 1 (201801/SOLO)", c.MatchedPeriods)
	}
}
