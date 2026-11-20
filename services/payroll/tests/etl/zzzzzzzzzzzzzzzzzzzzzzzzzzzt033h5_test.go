package etl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033h5: an orphan NIK (DETAIL without MAIN, D-35.2) whose PE_DATE_JOIN cannot be parsed holds only its own period
// (every row reported as an exception, nothing written), instead of failing the whole salary-legacy run. The other
// period in the same run still loads. Periods 201905/201906 SOLO are this test's own.
func TestT033h5_OrphanBadJoinDateHoldsOnlyItsPeriod(t *testing.T) {
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	exec(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
	 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`)
	exec(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid)
	 SELECT 1, c.pc_type, 1, 'SOLO', c.pc_sysid FROM payroll.pyr_component c WHERE c.pc_company_id = 1 AND c.pc_code = 'T001'
	 ON CONFLICT DO NOTHING`)
	t.Cleanup(func() {
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm IN (201905,201906) AND pr_division='SOLO'`)    //nolint:errcheck // cleanup
		_, _ = ownDB.Exec(`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm IN (201905,201906) AND pp_division='SOLO'`) //nolint:errcheck // cleanup
	})

	dir := t.TempDir()
	mainCols := strings.Split("PM_YYMM|PM_NIK|PM_NAMA|PM_DEPT|PM_GRADE|PM_JABATAN|PM_EMP_TYPE|PM_DATE|PM_PEMBAYARAN|PM_DATE_JOIN|PM_BASIC_PAY|PM_TUNJJABATAN|PM_TUNJRUMAH|PM_TUNJKESEJAHTERAAN|PM_TUNJTRANSPORT|PM_RATE_PERJAM|PM_TAX_CODE|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING|PM_DAY_PAY|PM_JAM_LEMBUR1|PM_JAM_LEMBUR2|PM_JAM_LEMBURNAS1|PM_JAM_LEMBURNAS2|PM_JAM_LEMBURNAS3|PM_JAM_LEMBUROFF1|PM_JAM_LEMBUROFF2|PM_JAM_LEMBUROFF3|PM_JAM_LEMBURNASP1|PM_JAM_LEMBURNASP2|PM_JAM_LEMBURNASP3|PM_DEPT_CODE|PM_REMARKS|PM_SECTION|PM_DIVISION", "|")
	detCols := strings.Split("PD_YYMM|PD_NIK|PD_NO|PD_TYPE|PD_DESCRIPTION|PD_AMOUNT|PD_STATUS|PD_TAX|PD_BASIS_AMOUNT|PD_REMARKS|PD_TAX_PAID_COMPANY|PD_DIVISION", "|")
	empCols := "PE_NIK|PE_DIVISION|PE_NAMA|PE_DATE_JOIN"
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	main := func(yymm, nik string) string {
		cells := map[string]string{"PM_YYMM": yymm, "PM_NIK": nik, "PM_DIVISION": "SOLO", "PM_DATE": "25-MAY-19", "PM_DATE_JOIN": "01-JAN-10",
			"PM_TOT_TUNJANGAN": "100", "PM_TOT_POTONGAN": "0", "PM_ROUNDING": "0"}
		out := make([]string, len(mainCols))
		for i, c := range mainCols {
			out[i] = cells[c]
		}
		return strings.Join(out, "|") + "\n"
	}
	det := func(yymm, nik, amt string) string {
		cells := map[string]string{"PD_YYMM": yymm, "PD_NIK": nik, "PD_NO": "1", "PD_TYPE": "Tunjangan", "PD_DIVISION": "SOLO", "PD_AMOUNT": amt,
			"PD_DESCRIPTION": "x", "PD_STATUS": "F", "PD_TAX": "2", "PD_TAX_PAID_COMPANY": "N"}
		out := make([]string, len(detCols))
		for i, c := range detCols {
			out[i] = cells[c]
		}
		return strings.Join(out, "|") + "\n"
	}
	write("MGT_PAYROLL_MAIN.psv", strings.Join(mainCols, "|")+"\n"+main("201905", "H5-OK")+main("201906", "H5-OK2"))
	write("MGT_PAYROLL_DETAIL.psv", strings.Join(detCols, "|")+"\n"+
		det("201905", "H5-OK", "100")+det("201906", "H5-OK2", "100")+
		det("201905", "H5-ORPHAN", "50")) // orphan in 201905 only; its EMP row has an unparsable join date
	write("MGT_PAYROLL_EMP.psv", empCols+"\nH5-ORPHAN|SOLO|Yatim|bukan-tanggal\n")

	reps, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.SalaryLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatalf("atribut MAIN sintetis rusak harus jadi exception per periode, bukan error run: %v", err)
	}
	var excM, excD []string
	for _, r := range reps {
		if !r.OK() {
			t.Fatalf("%s: laporan tidak seimbang src=%d dst=%d exc=%d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
		if strings.Contains(r.Table, "DETAIL") {
			excD = r.Exceptions
		} else {
			excM = r.Exceptions
		}
	}
	if len(excM) != 1 || len(excD) != 2 {
		t.Fatalf("201905 ditahan: mau 1 exception MAIN + 2 DETAIL, dapat %v / %v", excM, excD)
	}
	if !strings.Contains(strings.Join(excD, " "), "PE_DATE_JOIN") {
		t.Errorf("exception harus menyebut PE_DATE_JOIN: %v", excD)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=201905 AND pr_division='SOLO'`); got != "0" {
		t.Errorf("201905 tidak boleh ditulis, run=%s", got)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=201906 AND pr_division='SOLO' AND pr_kind='LEGACY'`); got != "1" {
		t.Errorf("201906 harus tetap dimuat, run=%s", got)
	}
}
