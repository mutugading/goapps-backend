package isolation

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// I24 (THR/Bonus, T024a, D-56): mutation of a CLOSED THR event (and its run/head/detail) as payroll_app is
// rejected by fn_guard_thr_closed (P0001). A LEGACY run (ETL history) stays writable by the guard.
func TestI24_ThrClosedEventMutationRejected(t *testing.T) {
	t.Cleanup(func() {
		// Owner may disable its own triggers; the guard would otherwise block removing the CLOSED fixture.
		for _, q := range []string{
			`ALTER TABLE payroll.pyr_thr_event DISABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_run DISABLE TRIGGER USER`,
			`UPDATE payroll.pyr_thr_event SET pte_active_run_id=NULL WHERE pte_description='i24'`,
			`DELETE FROM payroll.pyr_thr_run WHERE ptr_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_description='i24')`,
			`DELETE FROM payroll.pyr_thr_event WHERE pte_description='i24'`,
			`ALTER TABLE payroll.pyr_thr_event ENABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_run ENABLE TRIGGER USER`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v\n%s", err, q)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, created_by)
		 VALUES (1,'SOLO','THR',202603,'2026-03-10','i24','t')`,
		`INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status) SELECT pte_id,'NORMAL','SUPERSEDED' FROM payroll.pyr_thr_event WHERE pte_description='i24'`,
		`UPDATE payroll.pyr_thr_event SET pte_status='CLOSED' WHERE pte_description='i24'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("fixture: %v\n%s", err, q)
		}
	}
	ctx := ctxFor(t, userSolo)
	for _, tt := range []struct{ name, q string }{
		{"event update", `UPDATE payroll.pyr_thr_event SET pte_description='x' WHERE pte_description='i24'`},
		{"event delete", `DELETE FROM payroll.pyr_thr_event WHERE pte_description='i24'`},
		{"run update", `UPDATE payroll.pyr_thr_run SET ptr_emp_count=1 WHERE ptr_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_description='i24')`},
		{"run insert", `INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_status) SELECT pte_id,'SUPERSEDED' FROM payroll.pyr_thr_event WHERE pte_description='i24'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error { _, e := tx.ExecContext(ctx, tt.q); return e })
			assertPgCode(t, err, "P0001")
		})
	}
}

// T024a review: the LEGACY exception of fn_guard_thr_closed belongs to payroll_etl only. payroll_app can neither
// create a LEGACY run / CLOSED event nor mutate children of an event that has a LEGACY run; payroll_etl can load it.
func TestI24_ThrLegacyHistoryEtlOnly(t *testing.T) {
	const ev = `(SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_description='i24legacy')`
	const run = `(SELECT ptr_id FROM payroll.pyr_thr_run WHERE ptr_event_id=` + ev + ` AND ptr_kind='LEGACY')`
	t.Cleanup(func() {
		for _, q := range []string{
			`ALTER TABLE payroll.pyr_thr_event DISABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_run DISABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_head DISABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_detail DISABLE TRIGGER USER`,
			`UPDATE payroll.pyr_thr_event SET pte_active_run_id=NULL WHERE pte_description IN ('i24legacy','i24appclosed')`,
			`DELETE FROM payroll.pyr_thr_detail WHERE ptd_run_id IN (SELECT ptr_id FROM payroll.pyr_thr_run WHERE ptr_event_id=` + ev + `)`,
			`DELETE FROM payroll.pyr_thr_head WHERE pth_run_id IN (SELECT ptr_id FROM payroll.pyr_thr_run WHERE ptr_event_id=` + ev + `)`,
			`DELETE FROM payroll.pyr_thr_run WHERE ptr_event_id IN (SELECT pte_id FROM payroll.pyr_thr_event WHERE pte_description LIKE 'i24%')`,
			`DELETE FROM payroll.pyr_thr_event WHERE pte_description IN ('i24legacy','i24appclosed','i24app')`,
			`ALTER TABLE payroll.pyr_thr_event ENABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_run ENABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_head ENABLE TRIGGER USER`,
			`ALTER TABLE payroll.pyr_thr_detail ENABLE TRIGGER USER`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v\n%s", err, q)
			}
		}
	})
	var emp int64
	if err := ownDB.QueryRow(`SELECT pe_id FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_division='SOLO' LIMIT 1`).Scan(&emp); err != nil {
		t.Fatalf("employee: %v", err)
	}

	// payroll_etl: CLOSED event + LEGACY run + head/detail (detail without component sysid, D-56), then activate.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, pte_status, created_by)
		 VALUES (1,'SOLO','THR',202603,'2026-03-10','i24legacy','CLOSED','etl')`,
		`INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status) SELECT ` + ev + `,'LEGACY','ACTIVE'`,
		fmt.Sprintf(`INSERT INTO payroll.pyr_thr_head (pth_run_id, pth_employee_id, pth_nik, pth_tot_tunjangan, pth_tot_potongan, pth_rounding)
		 SELECT %s, %d, 'S1', 100, 0, 0`, run, emp),
		fmt.Sprintf(`INSERT INTO payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_nik, ptd_type, ptd_id, ptd_amount, ptd_meta)
		 SELECT %s, %d, 'S1', 'Tunjangan', 8, 100, '{"legacy_unmapped":true}'`, run, emp),
		`UPDATE payroll.pyr_thr_event SET pte_active_run_id=` + run + ` WHERE pte_description='i24legacy'`,
	} {
		if _, err := etlDB.Exec(q); err != nil {
			t.Fatalf("etl load: %v\n%s", err, q)
		}
	}
	// payroll_etl cannot read amounts (I10 pattern).
	var n int
	if err := etlDB.QueryRow(`SELECT count(*) FROM payroll.pyr_thr_head`).Scan(&n); err == nil {
		t.Fatal("etl must not SELECT pyr_thr_head")
	}
	if err := etlDB.QueryRow(`SELECT count(ptr_tot_tunjangan) FROM payroll.pyr_thr_run`).Scan(&n); err == nil {
		t.Fatal("etl must not SELECT ptr_tot_*")
	}

	ctx := ctxFor(t, userSolo)
	for _, tt := range []struct{ name, q string }{
		{"app insert LEGACY run into CLOSED event", `INSERT INTO payroll.pyr_thr_run (ptr_event_id, ptr_kind, ptr_status) SELECT ` + ev + `,'LEGACY','SUPERSEDED'`},
		{"app insert CLOSED event", `INSERT INTO payroll.pyr_thr_event (pte_company_id, pte_division, pte_kind, pte_yymm, pte_event_date, pte_description, pte_status, created_by)
		 VALUES (1,'SOLO','BONUS',202603,'2026-03-10','i24appclosed','CLOSED','app')`},
		{"app update head of LEGACY event", `UPDATE payroll.pyr_thr_head SET pth_rounding=1 WHERE pth_run_id=` + run},
		{"app delete head of LEGACY event", `DELETE FROM payroll.pyr_thr_head WHERE pth_run_id=` + run},
		{"app update detail of LEGACY event", `UPDATE payroll.pyr_thr_detail SET ptd_amount=1 WHERE ptd_run_id=` + run},
		{"app insert detail into LEGACY event", fmt.Sprintf(`INSERT INTO payroll.pyr_thr_detail (ptd_run_id, ptd_employee_id, ptd_nik, ptd_type, ptd_id, ptd_amount, ptd_meta)
		 SELECT %s, %d, 'S1', 'Tunjangan', 9, 1, '{"legacy_unmapped":true}'`, run, emp)},
		{"app update LEGACY event", `UPDATE payroll.pyr_thr_event SET pte_description='x' WHERE pte_description='i24legacy'`},
		{"app delete LEGACY run", `DELETE FROM payroll.pyr_thr_run WHERE ptr_id=` + run},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error { _, e := tx.ExecContext(ctx, tt.q); return e })
			if err == nil {
				t.Fatal("want rejection, got nil")
			}
		})
	}
	if got := queryOwner(t, `SELECT (SELECT count(*) FROM payroll.pyr_thr_detail WHERE ptd_run_id=`+run+`)::text||'/'||
		(SELECT sum(pth_tot_tunjangan) FROM payroll.pyr_thr_head WHERE pth_run_id=`+run+`)::text`); len(got) != 1 || got[0] != "1/100.00" {
		t.Fatalf("LEGACY data berubah: %v", got)
	}
	// Scope: a SOLO user sees the SOLO legacy rows through the compat view, a JAKARTA-only scope would not (RLS p_scope/p_parent).
	if got := queryStrings(t, ctx, `SELECT td_no::text FROM payroll.v_pyr_thr_detail WHERE td_nik='S1'`); strings.Join(got, ",") != "8" {
		t.Fatalf("SOLO view: %v", got)
	}
}
