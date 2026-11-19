package etl

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

const (
	h2RekapHdr = "MPAR_YYMM|MPAR_NIK|MPAR_NAMA|MPAR_DAY_WORK|MPAR_LEMBUR_1|MPAR_LEMBUR_2|MPAR_CUTI|MPAR_ALPA|MPAR_DIVISION\n"
	h2AddHdr   = "MPARA_YYMM|MPARA_NIK|MPARA_NAMA|MPARA_DAY_WORK|MPARA_LEMBUR_1|MPARA_DIVISION\n"
	h2SchHdr   = "MPSR_YYMM|MPSR_NIK|MPSR_SCHEDULE|MPSR_NO_SCH|MPSR_DIVISION\n"
)

func h2AttDir(t *testing.T, rekap string) etl.PSVDir {
	t.Helper()
	return inputsDir(t, map[string]string{
		etl.TableAttRekap: h2RekapHdr + rekap, etl.TableAttRekapAdd: h2AddHdr, etl.TableSchRekap: h2SchHdr,
	})
}

// T033h2 (review #6): a NIK that left the spool leaves the LEGACY snapshot of an open period; an APPROVED/CLOSED period
// keeps it (reported), and a yymm absent from the source is not touched at all.
func TestT033h2_AttLegacyPrunesStaleRows(t *testing.T) {
	ctx := context.Background()
	o := etl.AttLegacyOptions{CompanyID: 1}
	const rows = `SELECT coalesce(string_agg(r.pasr_nik, ',' ORDER BY r.pasr_nik), '') FROM payroll.pyr_att_snapshot_row r
JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id
WHERE s.pas_company_id=1 AND s.pas_division='SOLO' AND s.pas_source_batch_ids='{}' AND s.pas_yymm=`
	const kinds = `SELECT coalesce(string_agg(DISTINCT r.pasr_kind, ',' ORDER BY r.pasr_kind), '') FROM payroll.pyr_att_snapshot_row r
JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id
WHERE s.pas_company_id=1 AND s.pas_division='SOLO' AND s.pas_source_batch_ids='{}' AND s.pas_yymm=`
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=209702 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id IN (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_yymm IN (209701,209702))`,
			`DELETE FROM payroll.pyr_att_snapshot WHERE pas_yymm IN (209701,209702)`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
	full := "209701|H2-A|A|20|||||SOLO\n209701|H2-B|B|20|||||SOLO\n209702|H2-A|A|20|||||SOLO\n209702|H2-B|B|20|||||SOLO\n"
	if _, err := etl.RunAttLegacy(ctx, etlDB, h2AttDir(t, full), o); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
VALUES (1,209702,'SOLO','2097-01-26','2097-02-25','APPROVED')`); err != nil {
		t.Fatal(err)
	}
	reps, err := etl.RunAttLegacy(ctx, etlDB, h2AttDir(t, "209701|H2-A|A|20|||||SOLO\n209702|H2-A|A|20|||||SOLO\n"), o)
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, rows+"209701"); got != "H2-A" {
		t.Errorf("periode terbuka: baris %q, want H2-A (H2-B basi dihapus)", got)
	}
	if got := scalar(t, rows+"209702"); got != "H2-A,H2-B" {
		t.Errorf("periode APPROVED: baris %q, want H2-A,H2-B (tidak diubah)", got)
	}
	if !strings.Contains(strings.Join(reps[0].Warnings, "\n"), "209701/SOLO: 1 baris snapshot dihapus") ||
		!strings.Contains(strings.Join(reps[0].Skipped, "\n"), "209702/SOLO: periode APPROVED, sumber BERBEDA") {
		t.Errorf("warning = %v skipped = %v", reps[0].Warnings, reps[0].Skipped)
	}
	// An empty ADD/SCH spool (failed export) never prunes those kinds: only kinds with rows in the load are pruned.
	if _, err := etl.RunAttLegacy(ctx, etlDB, inputsDir(t, map[string]string{
		etl.TableAttRekap: h2RekapHdr + "209701|H2-A|A|20|||||SOLO\n", etl.TableAttRekapAdd: h2AddHdr + "209701|H2-A|A|1|1|SOLO\n", etl.TableSchRekap: h2SchHdr,
	}), o); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, h2AttDir(t, "209701|H2-A|A|20|||||SOLO\n"), o); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, kinds+"209701"); got != "ADD,REKAP" {
		t.Errorf("spool ADD kosong menghapus baris ADD: %q", got)
	}
	// ETL may delete rows of LEGACY snapshots only (policy p_etl_del), never of a sync (ENGINE) snapshot.
	if got := scalar(t, `SELECT pg_get_expr(polqual, polrelid) FROM pg_policy WHERE polname='p_etl_del' AND polrelid='payroll.pyr_att_snapshot_row'::regclass`); !strings.Contains(got, "pas_source_batch_ids = '{}'") {
		t.Errorf("p_etl_del = %q", got)
	}
	// A source that only carries 209701 leaves 209702 alone (prune is scoped to the yymm/division in the source).
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=209702 AND pp_division='SOLO'`); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, h2AttDir(t, "209701|H2-A|A|20|||||SOLO\n"), o); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, rows+"209702"); got != "H2-A,H2-B" {
		t.Errorf("yymm di luar sumber berubah: %q", got)
	}
}

// T033h2: an MPSP spool without valid rows does not retire the ETL subsidy rules (fail closed).
func TestT033h2_SubsidyEmptySourceFailsClosed(t *testing.T) {
	ctx := context.Background()
	ensureComponents(t)
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_subsidy_rule WHERE psr_name='ETL subsidi SOLO 209703'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if _, err := etl.RunInputs(ctx, etlDB, inputsDir(t, map[string]string{
		etl.TableSubsidiPPh: "MPSP_YYMM|MPSP_YN|MPSP_LIMIT_AMT|MPSP_DIVISION|MPSP_DEPT\n209703|Y|10000000|SOLO|PROD\n",
	}), etl.InputOptions{CompanyID: 1, AllowRetire: true}); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"kosong":      "",
		"semua kotor": "2027|Y|1|SOLO|PROD\n",
	} {
		_, err := etl.RunInputs(ctx, etlDB, inputsDir(t, map[string]string{
			etl.TableSubsidiPPh: "MPSP_YYMM|MPSP_YN|MPSP_LIMIT_AMT|MPSP_DIVISION|MPSP_DEPT\n" + body,
		}), etl.InputOptions{CompanyID: 1})
		if err == nil || !strings.Contains(err.Error(), "tidak di-retire") {
			t.Fatalf("%s: err = %v, want fail closed", name, err)
		}
	}
	if got := scalar(t, `SELECT psr_status FROM payroll.pyr_subsidy_rule WHERE psr_name='ETL subsidi SOLO 209703'`); got != "ACTIVE" {
		t.Fatalf("rule = %s, want ACTIVE", got)
	}
}

// T033h4: a truncated MPSP spool (still >=1 valid group) must not silently retire the ETL rules after the cut: the load
// fails and rolls back unless AllowRetire is given.
func TestT033h4_SubsidyPartialSourceNeedsAllowRetire(t *testing.T) {
	ctx := context.Background()
	ensureComponents(t)
	const hdr = "MPSP_YYMM|MPSP_YN|MPSP_LIMIT_AMT|MPSP_DIVISION|MPSP_DEPT\n"
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_subsidy_rule WHERE psr_name IN ('ETL subsidi SOLO 209704','ETL subsidi SOLO 209705')`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if _, err := etl.RunInputs(ctx, etlDB, inputsDir(t, map[string]string{
		etl.TableSubsidiPPh: hdr + "209704|Y|10000000|SOLO|PROD\n209705|Y|10000000|SOLO|PROD\n",
	}), etl.InputOptions{CompanyID: 1, AllowRetire: true}); err != nil { // AllowRetire: buang rule ETL sisa tes lain
		t.Fatal(err)
	}
	cut := inputsDir(t, map[string]string{etl.TableSubsidiPPh: hdr + "209704|Y|10000000|SOLO|PROD\n"})
	_, err := etl.RunInputs(ctx, etlDB, cut, etl.InputOptions{CompanyID: 1})
	if err == nil || !strings.Contains(err.Error(), "-allow-retire") {
		t.Fatalf("err = %v, want allow-retire hint", err)
	}
	const q = `SELECT psr_status FROM payroll.pyr_subsidy_rule WHERE psr_name='ETL subsidi SOLO 209705'`
	if got := scalar(t, q); got != "ACTIVE" {
		t.Fatalf("rule = %s, want ACTIVE (rolled back)", got)
	}
	if _, err := etl.RunInputs(ctx, etlDB, cut, etl.InputOptions{CompanyID: 1, AllowRetire: true}); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, q); got != "RETIRED" {
		t.Fatalf("rule = %s, want RETIRED", got)
	}
}

// T033h2: a template level HR edited in the app (marker in pyr_component_template_owner) is skipped by the ETL, so rows HR
// deleted are not resurrected; the other levels still load.
func TestT033h2_TemplateManagedLevelNotResurrected(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	opt := etl.Options{CompanyID: 1, ValidFrom: 202001}
	if _, err := etl.Run(ctx, etlDB, src, opt); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_component_template_owner WHERE pcto_template='Driver'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if _, err := etl.Run(ctx, etlDB, src, opt); err != nil {
			t.Errorf("cleanup rerun: %v", err)
		}
	})
	// HR emptied the Driver level in the app (what ReplaceTemplate does: marker + delete); an empty actor still counts.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component_template_owner (pcto_company_id, pcto_template, pcto_managed_by) VALUES (1,'Driver','')`,
		`DELETE FROM payroll.pyr_component_template WHERE pct_template='Driver'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	reps, err := etl.Run(ctx, etlDB, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_component_template WHERE pct_template='Driver'`); got != "0" {
		t.Fatalf("template Driver dihidupkan lagi oleh ETL: %s baris", got)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_component_template WHERE pct_template='Executive'`); got != "2" {
		t.Fatalf("template Executive = %s baris, want 2", got)
	}
	tr := reps[1]
	if tr.SkippedRows != 2 || tr.DstRows != 2 || !tr.OK() || len(tr.Skipped) != 1 || !strings.Contains(tr.Skipped[0], "template Driver") {
		t.Fatalf("report template %+v", tr)
	}
}

// T033h2: blank/unknown/conflicting DEPT_FIELD_01 clears the old group, and exceptions roll the load back unless allowed.
func TestT033h2_CostGroupClearsAndRollsBack(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_employee WHERE pe_nik='H2-APP'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_dept_id=NULL, pe_cost_group=NULL WHERE pe_company_id=1 AND pe_dept_id LIKE 'H2%'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	niks := []string{"1001", "1002", "2001"}
	for i, d := range []string{"H2BLANK", "H2BAD", "H2OK"} {
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_dept_id=$1, pe_cost_group='MGT', updated_by='ETL' WHERE pe_company_id=1 AND pe_nik=$2`, d, niks[i]); err != nil {
			t.Fatal(err)
		}
	}
	// H2-APP in H2BAD: a second employee whose group HR set in the app; the source conflict must not wipe it.
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_dept_id, pe_cost_group, updated_by)
VALUES (1,'H2-APP','SOLO','App','H2-APP','H2BAD','MB','hr-user')`); err != nil {
		t.Fatal(err)
	}
	hdr := "DEPT_COMP_CODE|DEPT_CODE|DEPT_FIELD_01\n"
	dir := writePSV(t, "FM_DEPARTMENT", hdr, "002|H2BLANK|", "002|H2BAD|MGT", "002|H2BAD|MB", "002|H2OK|MB")
	groups := func() string {
		return scalar(t, `SELECT string_agg(pe_dept_id||'='||coalesce(pe_cost_group,'-'), ',' ORDER BY pe_dept_id, coalesce(pe_cost_group,'-')) FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_dept_id LIKE 'H2%'`)
	}
	before := groups()
	rep, err := etl.RunCostGroup(ctx, etlDB, dir, etl.CostGroupOptions{CompanyID: 1})
	if err == nil || len(rep.Exceptions) != 1 {
		t.Fatalf("exception tanpa allow: err=%v exc=%v", err, rep.Exceptions)
	}
	if got := groups(); got != before {
		t.Fatalf("load dengan exception ter-commit: %s (sebelum %s)", got, before)
	}
	if _, err := etl.RunCostGroup(ctx, etlDB, dir, etl.CostGroupOptions{CompanyID: 1, AllowExceptions: true}); err != nil {
		t.Fatal(err)
	}
	if got, want := groups(), "H2BAD=-,H2BAD=MB,H2BLANK=-,H2OK=MB"; got != want {
		t.Fatalf("cost group = %s, want %s", got, want)
	}
}

// T033h2: a DETAIL row with a broken key holds back the event of its NIK (it may belong there), not only its own row.
func TestT033h2_ThrBrokenDetailHoldsEvent(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	thrEmployees(t) // yymm 209704 is unique to this test; the CLOSED event stays (THR guard, like the other THR tests)
	main := thrMain("209704", "THR", "THR", "10-APR-97", "1001", "300", "0", "0", "SOLO") +
		thrMain("209704", "THR", "THR JKT", "10-APR-97", "2001", "100", "0", "0", "JAKARTA")
	det := thrDet("209704", "THR", "1001", "1", "Tunjangan", "200", "SOLO") +
		thrDet("209704", "THR", "1001", "1", "Tunjangan", "100", "BANDUNG") + // division rusak: milik event SOLO?
		thrDet("209704", "THR", "2001", "1", "Tunjangan", "100", "JAKARTA")
	reps, err := etl.RunThrLegacy(ctx, etlDB, writeThr(t, main, det), etl.ThrLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT coalesce(string_agg(pte_division::text, ',' ORDER BY 1), '') FROM payroll.pyr_thr_event WHERE pte_yymm=209704`); got != "JAKARTA" {
		t.Fatalf("event dimuat = %q, want hanya JAKARTA (SOLO ditahan)", got)
	}
	m, d := reps[0], reps[1]
	if !m.OK() || !d.OK() || m.DstRows != 1 || d.DstRows != 1 || len(d.Exceptions) != 2 {
		t.Fatalf("main %+v\ndetail %+v", m, d)
	}
	if !strings.Contains(fmt.Sprint(m.Exceptions), "berkey rusak") {
		t.Fatalf("alasan tahan tidak menyebut baris rusak: %v", m.Exceptions)
	}
}
