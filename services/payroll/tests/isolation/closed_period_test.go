package isolation

import (
	"database/sql"
	"testing"
)

// I24 (09§6.5): direct SQL as payroll_app on a CLOSED period is rejected by fn_guard_closed (P0001)
// for result and addition rows, also within the caller's scope. The service layer (period.Guard,
// run commit) is covered by tests/etl TestPeriodTransitions. Period 202610 SOLO, separate from seed.
func TestI24_ClosedPeriodMutationRejected(t *testing.T) {
	// Other tests count the seed rows of SOLO: reopen as owner and remove the fixture afterwards.
	t.Cleanup(func() {
		for _, q := range []string{
			`UPDATE payroll.pyr_period SET pp_status='OPEN', pp_active_run_id=NULL WHERE pp_company_id=1 AND pp_yymm=202610 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_addition WHERE pa_company_id=1 AND pa_yymm=202610 AND pa_division='SOLO'`,
			`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202610 AND pr_division='SOLO'`,
			`DELETE FROM payroll.pyr_att_snapshot WHERE pas_checksum='i24'`,
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202610 AND pp_division='SOLO'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v\n%s", err, q)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (1,202610,'SOLO','2026-09-26','2026-10-25')`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum) VALUES (1,202610,'SOLO','{1}','i24')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
		 SELECT 1,202610,'SOLO','2026-10-25',1,pas_snapshot_id,'ACTIVE' FROM payroll.pyr_att_snapshot WHERE pas_checksum='i24'`,
		`INSERT INTO payroll.pyr_salary_head (psh_company_id, psh_run_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
		 SELECT 1, pr_id, 'S1', 202610, 'SOLO', 100, 0, 0 FROM payroll.pyr_run WHERE pr_yymm=202610 AND pr_division='SOLO'`,
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		 SELECT pr_id, 'S1', 'Tunjangan', 1, 100 FROM payroll.pyr_run WHERE pr_yymm=202610 AND pr_division='SOLO'`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount) VALUES (1,202610,'S1','SOLO','Tunjangan',5,10)`,
		`UPDATE payroll.pyr_period SET pp_status='CLOSED', pp_active_run_id=(SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=202610 AND pr_division='SOLO')
		 WHERE pp_company_id=1 AND pp_yymm=202610 AND pp_division='SOLO'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("fixture: %v\n%s", err, q)
		}
	}
	const run = `(SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=202610 AND pr_division='SOLO')`
	ctx := ctxFor(t, userSolo)
	for _, tt := range []struct{ name, q string }{
		{"detail update", `UPDATE payroll.pyr_salary_detail SET psd_amount = 1 WHERE psd_run_id = ` + run},
		{"detail insert", `INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount) SELECT ` + run + `, 'S1', 'Tunjangan', 2, 1`},
		{"detail delete", `DELETE FROM payroll.pyr_salary_detail WHERE psd_run_id = ` + run},
		{"header update", `UPDATE payroll.pyr_salary_head SET psh_rounding = 1 WHERE psh_run_id = ` + run},
		{"header delete", `DELETE FROM payroll.pyr_salary_head WHERE psh_run_id = ` + run},
		{"addition insert", `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount) VALUES (1,202610,'S1','SOLO','Tunjangan',6,1)`},
		{"addition update", `UPDATE payroll.pyr_addition SET pa_amount = 99 WHERE pa_yymm = 202610 AND pa_division = 'SOLO'`},
		{"addition delete", `DELETE FROM payroll.pyr_addition WHERE pa_yymm = 202610 AND pa_division = 'SOLO'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error { _, e := tx.ExecContext(ctx, tt.q); return e })
			assertPgCode(t, err, "P0001")
		})
	}
	if got := queryOwner(t, `SELECT (SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_run_id = `+run+`)::text||'/'||
		(SELECT sum(pa_amount) FROM payroll.pyr_addition WHERE pa_yymm = 202610 AND pa_division = 'SOLO')::text`); len(got) != 1 || got[0] != "1/10.00" {
		t.Fatalf("data CLOSED berubah: %v", got)
	}
}
