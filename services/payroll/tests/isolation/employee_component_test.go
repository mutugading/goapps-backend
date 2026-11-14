package isolation

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// Attachments (and their history) follow the employee through RLS: a SOLO user sees
// the SOLO employee's rows, never JAKARTA's or another company's, and cannot attach
// to an out-of-scope employee (T008d).
func TestRLS_EmployeeComponentFollowsEmployeeScope(t *testing.T) {
	ctx := context.Background()
	for _, s := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from, pc_status) VALUES
		   (1,'T950','Tunjangan','T950',202601,'ACTIVE'), (2,'T950','Tunjangan','T950',202601,'ACTIVE')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_amount, pec_source)
		   SELECT e.pe_company_id, e.pe_nik, e.pe_division, c.pc_sysid, 1, 'TEMPLATE'
		     FROM payroll.pyr_employee e JOIN payroll.pyr_component c ON c.pc_company_id = e.pe_company_id AND c.pc_code='T950'`,
	} {
		if _, err := ownDB.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, s := range []string{
			`DELETE FROM payroll.pyr_employee_component WHERE pec_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='T950')`,
			`DELETE FROM payroll.pyr_component WHERE pc_code='T950'`,
		} {
			if _, err := ownDB.ExecContext(ctx, s); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})

	const q = `SELECT pec_company_id::text || ':' || pec_division || ':' || pec_nik FROM payroll.pyr_employee_component ORDER BY 1`
	if got := queryStrings(t, ctxFor(t, userSolo), q); strings.Join(got, ",") != "1:SOLO:S1" {
		t.Fatalf("solo sees %v, want only company 1 SOLO", got)
	}
	if got := queryStrings(t, ctxFor(t, userTwo), q); strings.Join(got, ",") != "1:JAKARTA:J1,1:SOLO:S1" {
		t.Fatalf("two-division user sees %v", got)
	}
	if got := queryStrings(t, ctxFor(t, userNone), q); len(got) != 0 {
		t.Fatalf("no-scope user sees %v, want none", got)
	}

	// WITH CHECK / component RLS: a SOLO user cannot attach to the JAKARTA employee nor to company 2.
	// The company 2 component is invisible, so that INSERT may match no row instead of erroring;
	// what matters is that nothing lands.
	sctx := ctxFor(t, userSolo)
	for _, tc := range []struct{ company, nik, div string }{{"1", "J1", "JAKARTA"}, {"2", "O1", "SOLO"}} {
		_ = appDB.ScopedTx(sctx, func(tx *sql.Tx) error { //nolint:errcheck // error or no-op are both acceptable here
			_, err := tx.ExecContext(sctx, `INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_amount)
			  SELECT $1::smallint, $2, $3::payroll.division, pc_sysid, 5 FROM payroll.pyr_component WHERE pc_code='T950' AND pc_company_id=$1::smallint
			  ON CONFLICT (pec_company_id, pec_nik, pec_division, pec_component_sysid) DO UPDATE SET pec_amount = 5`, tc.company, tc.nik, tc.div)
			return err
		})
	}
	var leaked int
	if err := ownDB.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_employee_component WHERE pec_amount = 5`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("out-of-scope attach/update landed %d rows (%v)", leaked, err)
	}
}
