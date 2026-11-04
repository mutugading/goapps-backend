package etl

import (
	"context"
	"fmt"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T017d (D-40): the legacy attendance ETL leaves the snapshot of a period with an ACTIVE run untouched
// (checksum and lock), and still refreshes the snapshot of periods without one.
func TestETL_AttLegacySkipsPeriodWithActiveRun(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.AttLegacyOptions{CompanyID: 1, ExpectRekap: 5, ExpectAdd: 1, ExpectS: 3}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	const snapWhere = `WHERE pas_company_id=1 AND pas_yymm=202607 AND pas_division='SOLO' AND pas_source_batch_ids='{}'`
	const snap = `FROM payroll.pyr_att_snapshot ` + snapWhere
	orig := scalar(t, `SELECT pas_checksum||'/'||pas_locked::text `+snap)
	hasRun := scalar(t, `SELECT EXISTS (SELECT 1 FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_status='ACTIVE' AND pr_kind='ENGINE')::text`)
	var insertedRun int64
	if hasRun != "true" {
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (1,202607,'SOLO','2026-06-26','2026-07-25') ON CONFLICT DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
			 SELECT 1,202607,'SOLO','2026-07-25',1,pas_snapshot_id,'ACTIVE' ` + snap + ` RETURNING pr_id`).Scan(&insertedRun); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if insertedRun != 0 { // only the run this test created, never another test's ENGINE run
			for _, q := range []string{
				fmt.Sprintf(`DELETE FROM payroll.pyr_run WHERE pr_id=%d`, insertedRun),
				`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202607 AND pp_division='SOLO' AND pp_active_run_id IS NULL`,
			} {
				if _, err := ownDB.Exec(q); err != nil {
					t.Errorf("cleanup: %v", err)
				}
			}
		}
		if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
			t.Errorf("cleanup rerun: %v", err)
		}
	})
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot SET pas_checksum='keep', pas_locked=false ` + snapWhere); err != nil {
		t.Fatal(err)
	}
	// 202606 JAKARTA has no run: it must still be refreshed by the same ETL run.
	if scalar(t, `SELECT EXISTS (SELECT 1 FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202606 AND pr_division='JAKARTA' AND pr_kind='ENGINE')::text`) == "true" {
		t.Fatal("fixture berubah: 202606 JAKARTA sudah punya run")
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot SET pas_checksum='stale' WHERE pas_company_id=1 AND pas_yymm=202606 AND pas_division='JAKARTA' AND pas_source_batch_ids='{}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT pas_checksum||'/'||pas_locked::text `+snap); got != "keep/false" {
		t.Fatalf("snapshot periode ber-run diubah ETL: %s (asli %s)", got, orig)
	}
	if got := scalar(t, `SELECT (pas_checksum <> 'stale')::text||'/'||pas_locked::text FROM payroll.pyr_att_snapshot WHERE pas_company_id=1 AND pas_yymm=202606 AND pas_division='JAKARTA' AND pas_source_batch_ids='{}'`); got != "true/true" {
		t.Fatalf("snapshot tanpa run tidak diperbarui: %s", got)
	}
}

// T017e (D-40): att_legacy_fail leaves REKAP rows of a period with an ACTIVE ENGINE run alone.
func TestETL_AttLegacyFailSkipsPeriodWithActiveRun(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.RunAttLegacy(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	const rows = `FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id
		WHERE s.pas_company_id=1 AND s.pas_yymm=202606 AND s.pas_division='JAKARTA' AND s.pas_source_batch_ids='{}' AND r.pasr_kind='REKAP'`
	var runID int64
	var periodAdded bool
	if scalar(t, `SELECT count(*) FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202606 AND pp_division='JAKARTA'`) == "0" {
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (1,202606,'JAKARTA','2026-05-26','2026-06-25')`); err != nil {
			t.Fatal(err)
		}
		periodAdded = true
	}
	if scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202606 AND pr_division='JAKARTA' AND pr_status='ACTIVE' AND pr_kind='ENGINE'`) == "0" {
		if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
			SELECT 1,202606,'JAKARTA','2026-06-25',1,pas_snapshot_id,'ACTIVE' FROM payroll.pyr_att_snapshot
			WHERE pas_company_id=1 AND pas_yymm=202606 AND pas_division='JAKARTA' AND pas_source_batch_ids='{}' RETURNING pr_id`).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { // only the run created here, never another test's ENGINE run
			if _, err := ownDB.Exec(fmt.Sprintf(`DELETE FROM payroll.pyr_run WHERE pr_id=%d`, runID)); err != nil {
				t.Errorf("cleanup: %v", err)
			}
			if periodAdded {
				if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202606 AND pp_division='JAKARTA' AND pp_active_run_id IS NULL`); err != nil {
					t.Errorf("cleanup period: %v", err)
				}
			}
		})
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot_row r SET pasr_premi_fail=7, pasr_beras_fail=7 FROM payroll.pyr_att_snapshot s
		WHERE s.pas_snapshot_id = r.pasr_snapshot_id AND s.pas_company_id=1 AND s.pas_yymm=202606 AND s.pas_division='JAKARTA' AND s.pas_source_batch_ids='{}' AND r.pasr_kind='REKAP'`); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*) `+rows+` AND r.pasr_premi_fail=7`); got == "0" {
		t.Fatal("fixture: tidak ada baris REKAP 202606 JAKARTA")
	}
	if _, err := etl.RunAttLegacyFail(ctx, etlDB, etl.PSVDir{Dir: "testdata_fail"}); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*) `+rows+` AND (r.pasr_premi_fail IS DISTINCT FROM 7 OR r.pasr_beras_fail IS DISTINCT FROM 7)`); got != "0" {
		t.Fatalf("%s baris REKAP periode ber-run ENGINE diubah ETL fail", got)
	}
}
