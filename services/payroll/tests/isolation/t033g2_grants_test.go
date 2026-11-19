package isolation

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// T033g2 (review PR #283 #2, D-77 (3)): payroll_ro is a NOLOGIN group read across divisions by policy p_ro, so
// app.scopes is neither needed nor able to widen or narrow what the read role sees; its login payroll_recon is a
// member. payroll_app cannot write formula/subsidy config; payroll_etl writes only its own source rows.

func roCount(t *testing.T, ro *sql.DB, scopes, q string) int {
	t.Helper()
	ctx := context.Background()
	tx, err := ro.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // read-only tx
	if scopes != "-" {
		if _, err := tx.ExecContext(ctx, `SELECT set_config('app.scopes', $1, true)`, scopes); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := tx.QueryRowContext(ctx, q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestT033g2_ReadRoleIgnoresScopeGUC(t *testing.T) {
	ro := openAs(t, "payroll_recon", "payroll_recon123", "payroll_db")
	var want int
	if err := ownDB.QueryRow(`SELECT count(*) FROM payroll.pyr_employee`).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if want < 3 {
		t.Fatalf("fixture: want employees in >= 2 companies, got %d", want)
	}
	// No GUC, a narrow GUC, and a forged wide GUC all see the same rows: scope is not the read role's boundary.
	for _, sc := range []string{"-", "", "1:SOLO", "1:SOLO,1:JAKARTA,2:SOLO,9:X"} {
		if got := roCount(t, ro, sc, `SELECT count(*) FROM payroll.pyr_employee`); got != want {
			t.Errorf("app.scopes=%q: ro sees %d employees, want %d", sc, got, want)
		}
	}
	// Every policy that names payroll_ro is the read-only p_ro (no GUC-based policy left for the read role).
	var bad []string
	rows, err := ownDB.Query(`SELECT c.relname || '.' || p.polname FROM pg_policy p JOIN pg_class c ON c.oid = p.polrelid
	  WHERE 'payroll_ro'::regrole::oid = ANY (p.polroles)
	    AND (p.polname <> 'p_ro' OR p.polcmd <> 'r' OR cardinality(p.polroles) <> 1 OR pg_get_expr(p.polqual, p.polrelid) <> 'true')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		bad = append(bad, s)
	}
	if len(bad) != 0 {
		t.Fatalf("policies for payroll_ro other than p_ro USING (true): %v", bad)
	}
	// The login is only a member; the group itself cannot log in; tables without p_ro stay closed (idempotency).
	var roLogin, recInRo bool
	if err := ownDB.QueryRow(`SELECT (SELECT rolcanlogin FROM pg_roles WHERE rolname = 'payroll_ro'),
	    pg_has_role('payroll_recon', 'payroll_ro', 'USAGE')`).Scan(&roLogin, &recInRo); err != nil {
		t.Fatal(err)
	}
	if roLogin || !recInRo {
		t.Fatalf("payroll_ro login=%v, payroll_recon member=%v", roLogin, recInRo)
	}
	var n int
	err = ro.QueryRow(`SELECT count(*) FROM payroll.pyr_idempotency_key`).Scan(&n)
	if err == nil && n != 0 {
		t.Fatalf("ro reads idempotency rows: %d", n)
	}
}

// App sessions keep the scope as a security boundary (unchanged): the shared policies now name payroll_app only.
func TestT033g2_AppStillScoped(t *testing.T) {
	got := queryStrings(t, ctxFor(t, userSolo), `SELECT pe_nik FROM payroll.pyr_employee ORDER BY 1`)
	if len(got) != 1 || got[0] != "S1" {
		t.Fatalf("app SOLO user sees %v, want [S1]", got)
	}
}

func TestT033g2_AppCannotWriteFormulaOrSubsidy(t *testing.T) {
	ctx := ctxFor(t, userTwo)
	for _, q := range []string{
		`INSERT INTO payroll.pyr_formula_version (pfv_company_id, pfv_component_sysid, pfv_division, pfv_valid_from, pfv_expression, created_by)
		 SELECT 1, pc_sysid, 'SOLO', 202601, '1', 'x' FROM payroll.pyr_component WHERE pc_company_id = 1 LIMIT 1`,
		`UPDATE payroll.pyr_formula_version SET pfv_status = 'ACTIVE'`,
		`UPDATE payroll.pyr_subsidy_rule SET psr_status = 'ACTIVE'`,
		`DELETE FROM payroll.pyr_subsidy_rule`,
		`INSERT INTO payroll.pyr_subsidy_override (pso_yymm, pso_company_id, pso_division, pso_nik, pso_action, pso_reason, created_by)
		 VALUES (202609, 1, 'SOLO', 'S1', 'EXCLUDE', 'x', 'x')`,
		`UPDATE payroll.pyr_subsidy_override SET approved_by = 'x'`,
	} {
		err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, q)
			return e
		})
		expectDenied(t, "app "+q[:min(40, len(q))], err)
	}
}

// ETL writes only rows of its own source: pyr_addition LEGACY, pyr_subsidy_rule created_by 'etl', legacy rule set 1.
func TestT033g2_EtlWritesOnlyOwnRows(t *testing.T) {
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM payroll.pyr_addition WHERE pa_nik IN ('G2A','G2B')`,
			`DELETE FROM payroll.pyr_subsidy_rule WHERE psr_name LIKE 'g2 %'`,
			`DELETE FROM payroll.pyr_rule_constant WHERE prc_key LIKE 'G2_%'`,
			`DELETE FROM payroll.pyr_rule_set WHERE prs_id = 92`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup %s: %v", q, err)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'P977','Potongan','g2 subsidi','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_source, created_by)
		 VALUES (1, 209901, 'G2A', 'SOLO', 'Tunjangan', 1, 1, 'MANUAL', 'user')`,
		`INSERT INTO payroll.pyr_subsidy_rule (psr_company_id, psr_name, psr_valid_from, psr_basis_kind, psr_limit_amount,
		   psr_subsidy_kind, psr_target_component_sysid, psr_status, created_by)
		 SELECT 1, 'g2 app', 209901, 'COMPONENTS_PERIOD', 1, 'FULL_PPH', min(pc_sysid), 'DRAFT', 'maker' FROM payroll.pyr_component WHERE pc_code = 'P977'`,
		`INSERT INTO payroll.pyr_subsidy_rule (psr_company_id, psr_name, psr_valid_from, psr_basis_kind, psr_limit_amount,
		   psr_subsidy_kind, psr_target_component_sysid, psr_status, created_by)
		 SELECT 1, 'g2 etl', 209901, 'COMPONENTS_PERIOD', 1, 'FULL_PPH', min(pc_sysid), 'ACTIVE', 'etl' FROM payroll.pyr_component WHERE pc_code = 'P977'`,
		`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from) VALUES (92, 'g2 v2', 209901)`,
		`INSERT INTO payroll.pyr_rule_constant (prc_company_id, prc_rule_set_id, prc_key, prc_value) VALUES (1, 92, 'G2_APP', 1)`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("fixture %s: %v", q[:min(40, len(q))], err)
		}
	}
	mustRows := func(what, q string, want int64) {
		t.Helper()
		res, err := etlDB.Exec(q)
		if err != nil {
			if want == 0 {
				expectDenied(t, what, err)
				return
			}
			t.Fatalf("%s: %v", what, err)
		}
		if n, _ := res.RowsAffected(); n != want { //nolint:errcheck // pgx always reports RowsAffected
			t.Fatalf("%s: %d rows, want %d", what, n, want)
		}
	}
	// Not ETL rows: UPDATE matches nothing (USING), INSERT is rejected (WITH CHECK), DELETE has no grant.
	mustRows("upd manual addition", `UPDATE payroll.pyr_addition SET pa_amount = 9 WHERE pa_nik = 'G2A'`, 0)
	mustRows("upd app subsidy", `UPDATE payroll.pyr_subsidy_rule SET psr_status = 'ACTIVE' WHERE psr_name = 'g2 app'`, 0)
	mustRows("upd v2 constant", `UPDATE payroll.pyr_rule_constant SET prc_value = 9 WHERE prc_key = 'G2_APP'`, 0)
	for what, q := range map[string]string{
		"ins manual addition": `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_source)
		 VALUES (1, 209901, 'G2B', 'SOLO', 'Tunjangan', 1, 1, 'MANUAL')`,
		"ins app subsidy": `INSERT INTO payroll.pyr_subsidy_rule (psr_company_id, psr_name, psr_valid_from, psr_basis_kind, psr_limit_amount,
		   psr_subsidy_kind, psr_target_component_sysid, psr_status, created_by)
		 SELECT 1, 'g2 forged', 209901, 'COMPONENTS_PERIOD', 1, 'FULL_PPH', min(pc_sysid), 'ACTIVE', 'maker' FROM payroll.pyr_component WHERE pc_code = 'P977'`,
		"ins v2 constant": `INSERT INTO payroll.pyr_rule_constant (prc_company_id, prc_rule_set_id, prc_key, prc_value) VALUES (1, 92, 'G2_X', 1)`,
		"del addition":    `DELETE FROM payroll.pyr_addition WHERE pa_nik = 'G2A'`,
		// USING matches the ETL row, WITH CHECK rejects handing it to another owner.
		"retag etl row": `UPDATE payroll.pyr_subsidy_rule SET created_by = 'maker' WHERE psr_name = 'g2 etl'`,
	} {
		_, err := etlDB.Exec(q)
		expectDenied(t, what, err)
	}
	// ETL rows still load (the ETL path itself is covered by tests/etl).
	mustRows("ins legacy addition", `INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount, pa_source)
		 VALUES (1, 209901, 'G2B', 'SOLO', 'Tunjangan', 1, 1, 'LEGACY')`, 1)
	mustRows("upd legacy addition", `UPDATE payroll.pyr_addition SET pa_amount = 2 WHERE pa_nik = 'G2B'`, 1)
	mustRows("retire etl subsidy", `UPDATE payroll.pyr_subsidy_rule SET psr_status = 'RETIRED' WHERE psr_name = 'g2 etl'`, 1)
}

// T033g5: the child tables of a formula version (dependencies, test cases) are config too: p_parent only checks that the
// version exists, so without REVOKE the app could attach rows to a version it cannot write.
func TestT033g5_AppCannotWriteFormulaChildren(t *testing.T) {
	var vid, sysid int64
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_formula_version (pfv_company_id, pfv_component_sysid, pfv_division, pfv_valid_from, pfv_expression, created_by)
	  SELECT 1, pc_sysid, 'SOLO', 209801, '1', 'x' FROM payroll.pyr_component WHERE pc_company_id = 1 ORDER BY pc_sysid LIMIT 1
	  RETURNING pfv_id, pfv_component_sysid`).Scan(&vid, &sysid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_formula_version WHERE pfv_id = $1`, vid); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	ctx := ctxFor(t, userTwo)
	for name, q := range map[string]string{
		"insert dependency": `INSERT INTO payroll.pyr_formula_dependency (pfd_formula_version_id, pfd_depends_on_sysid) VALUES ($1, $2)`,
		"insert test case":  `INSERT INTO payroll.pyr_formula_test_case (pftc_formula_version_id, pftc_input) VALUES ($1, '{}'::jsonb)`,
		"update test case":  `UPDATE payroll.pyr_formula_test_case SET pftc_expected = 1 WHERE pftc_formula_version_id = $1`,
		"delete dependency": `DELETE FROM payroll.pyr_formula_dependency WHERE pfd_formula_version_id = $1`,
	} {
		args := []any{vid}
		if strings.Contains(q, "$2") {
			args = append(args, sysid)
		}
		err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, q, args...)
			return e
		})
		expectDenied(t, "app "+name, err)
	}
}
