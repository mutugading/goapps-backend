package isolation

import (
	"database/sql"
	"testing"
)

// I26 (T028a, D-62): tabel dokumen & distribusi email. Batch/item ikut scope company:division, item tidak bisa
// di-DELETE payroll_app (jejak audit), template/policy/account read-only untuk payroll_app.
func TestI26_DocDistributionScopeAndGrants(t *testing.T) {
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM payroll.pyr_distribution_item WHERE pdi_nik='S1'`,
			`DELETE FROM payroll.pyr_distribution_batch WHERE created_by='i26'`,
			`DELETE FROM payroll.pyr_mail_template WHERE created_by='i26'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v\n%s", err, q)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, created_by)
		 VALUES (1,'SLIP_GAJI',99,'s','b','i26')`,
		`INSERT INTO payroll.pyr_distribution_batch (pdb_company_id, pdb_division, pdb_doc_type, pdb_yymm, created_by)
		 VALUES (1,'JAKARTA','SLIP_GAJI',202609,'i26')`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("fixture: %v\n%s", err, q)
		}
	}
	ctx := ctxFor(t, userSolo)
	run := func(q string) error {
		return appDB.ScopedTx(ctx, func(tx *sql.Tx) error { _, e := tx.ExecContext(ctx, q); return e })
	}
	count := func(q string) int {
		var n int
		err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error { return tx.QueryRowContext(ctx, q).Scan(&n) })
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}

	// Scope: batch JAKARTA tidak terlihat oleh user SOLO; insert JAKARTA ditolak; insert SOLO lolos.
	if n := count(`SELECT count(*) FROM payroll.pyr_distribution_batch WHERE created_by='i26'`); n != 0 {
		t.Fatalf("batch JAKARTA visible to SOLO user: %d", n)
	}
	assertPgCode(t, run(`INSERT INTO payroll.pyr_distribution_batch (pdb_company_id, pdb_division, pdb_doc_type, created_by)
		VALUES (1,'JAKARTA','SLIP_GAJI','i26')`), "42501")
	if err := run(`INSERT INTO payroll.pyr_distribution_batch (pdb_company_id, pdb_division, pdb_doc_type, pdb_yymm, created_by)
		VALUES (1,'SOLO','SLIP_GAJI',202609,'i26')`); err != nil {
		t.Fatalf("insert batch in scope: %v", err)
	}
	// Item: insert lewat batch SOLO lolos (p_parent); DELETE ditolak (tidak ada grant).
	if err := run(`INSERT INTO payroll.pyr_distribution_item (pdi_batch_id, pdi_employee_id, pdi_nik)
		SELECT b.pdb_id, e.pe_id, e.pe_nik FROM payroll.pyr_distribution_batch b, payroll.pyr_employee e
		WHERE b.created_by='i26' AND e.pe_nik='S1'`); err != nil {
		t.Fatalf("insert item in scope: %v", err)
	}
	assertPgCode(t, run(`DELETE FROM payroll.pyr_distribution_item WHERE pdi_nik='S1'`), "42501")
	// Template: terlihat (company 1) tetapi payroll_app tidak bisa menulis.
	if n := count(`SELECT count(*) FROM payroll.pyr_mail_template WHERE created_by='i26'`); n != 1 {
		t.Fatalf("mail template visible = %d, want 1", n)
	}
	assertPgCode(t, run(`UPDATE payroll.pyr_mail_template SET pmt_status='ACTIVE' WHERE created_by='i26'`), "42501")
	assertPgCode(t, run(`INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, created_by)
		VALUES (1,'2030-01-01','i26')`), "42501")
}
