package isolation

import (
	"context"
	"testing"
)

// Two companies may reuse the same division code and business key (04§14):
// company 2 SOLO gets the same NIK/period/run key as company 1 SOLO, and the
// CLOSED guard only looks at the period of its own company.
func TestCompanyKeys_SameDivisionTwoCompanies(t *testing.T) {
	ctx := context.Background()
	tx, err := ownDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil {
			t.Errorf("rollback: %v", rbErr)
		}
	}()
	stmts := []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_orion_sub_acnt) VALUES (2,'S1','SOLO','Other S1','x')`,
		`INSERT INTO payroll.pyr_component_master (pcm_type, pcm_id, pcm_name) VALUES ('Tunjangan',1,'T1')`,
		// Attachments are keyed by component sysid (D-20); each company owns its own component row.
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from, pc_status) VALUES
		   (1,'T901','Tunjangan','T901',202601,'ACTIVE'), (2,'T901','Tunjangan','T901',202601,'ACTIVE')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_amount)
		   SELECT pc_company_id,'S1','SOLO',pc_sysid,1 FROM payroll.pyr_component WHERE pc_code='T901'`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,202609,'SOLO',25), (2,202609,'SOLO',25)`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status) VALUES
		   (2,202609,'SOLO','2026-09-01','2026-09-30','CLOSED')`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum) VALUES (2,202609,'SOLO','{1}','c')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
		   SELECT 2,202609,'SOLO','2026-09-30',1,max(pas_snapshot_id),'ACTIVE' FROM payroll.pyr_att_snapshot`,
		// Company 1 SOLO stays OPEN although company 2 SOLO is CLOSED.
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_type, pa_id, pa_division, pa_amount) VALUES (1,202609,'S1','Tunjangan',1,'SOLO',1)`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			t.Fatalf("%q: %v", s[:min(70, len(s))], err)
		}
	}
	// Run key must still point to an existing period of the same company.
	if _, err := tx.ExecContext(ctx, `SAVEPOINT a`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
		VALUES (2,202609,'JAKARTA','2026-09-30',1,1,'RUNNING')`)
	assertPgCode(t, err, "23503")
	if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT a`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_type, pa_id, pa_division, pa_amount) VALUES (2,202609,'S1','Tunjangan',1,'SOLO',1)`)
	assertPgCode(t, err, "P0001") // company 2 period is CLOSED
}
