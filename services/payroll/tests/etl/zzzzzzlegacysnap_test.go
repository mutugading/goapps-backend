package etl

import (
	"context"
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
	var inserted bool
	if hasRun != "true" {
		for _, q := range []string{
			`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (1,202607,'SOLO','2026-06-26','2026-07-25') ON CONFLICT DO NOTHING`,
			`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
			 SELECT 1,202607,'SOLO','2026-07-25',1,pas_snapshot_id,'ACTIVE' ` + snap,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatalf("%v\n%s", err, q)
			}
		}
		inserted = true
	}
	t.Cleanup(func() {
		if inserted {
			for _, q := range []string{
				`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_kind='ENGINE'`,
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
