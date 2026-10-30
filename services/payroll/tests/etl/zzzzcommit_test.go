package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T017: commit run against PG18 with the real engine and rule set 1 (ETL + full static values,
// TestRuleSetLoader_FromETL runs earlier in file order). Period 203203 SOLO, 1.300 employees.
const (
	commitYYMM = 203203
	commitEmps = 1300
)

func seedCommit(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001), (1,'P001','Potongan','PPh 21','ACTIVE',202001)
		 ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_dept)
		 SELECT 1, 'C' || lpad(g::text, 5, '0'), 'SOLO', 'Karyawan ' || g, 'Staff', 'TK', 2, 'C' || g, 'PROD' FROM generate_series(1, $1) g`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1, 'C' || lpad(g::text, 5, '0'), 'SOLO', c.pc_sysid, 'F', 2, 3000000 + g * 1000, 2
		 FROM generate_series(1, $1) g JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'T001'`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1, 'C' || lpad(g::text, 5, '0'), 'SOLO', c.pc_sysid, 'F', 0, 0, 2
		 FROM generate_series(1, $1) g JOIN payroll.pyr_component c ON c.pc_company_id = 1 AND c.pc_code = 'P001'`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203203,'SOLO',26), (1,203204,'SOLO',26)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,203203,'SOLO','{1}','commit',true), (1,203204,'SOLO','{1}','commit',true)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT s.pas_snapshot_id, 'REKAP', 'C' || lpad(g::text, 5, '0'), '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26,"MPAR_LEMBUR_1":2}'::jsonb
		 FROM payroll.pyr_att_snapshot s, generate_series(1, $1) g
		 WHERE s.pas_company_id = 1 AND s.pas_division = 'SOLO' AND s.pas_checksum = 'commit'`,
	} {
		var err error
		if strings.Contains(q, "$1") {
			_, err = ownDB.Exec(q, commitEmps)
		} else {
			_, err = ownDB.Exec(q)
		}
		if err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
}

// ruleFunc wires the T016a rule set loader into the run service (the server does the same).
func ruleFunc(ctx context.Context, tx *sql.Tx, q run.Query, ruleSetID int) (run.Rules, error) {
	rr, err := postgres.NewRuleSetLoader().Load(ctx, tx, postgres.RuleQuery{RuleSetID: ruleSetID, CompanyID: q.CompanyID,
		Division: q.Division, YYMM: q.YYMM})
	if postgres.IsRuleError(err) {
		return run.Rules{}, fmt.Errorf("%w: %w", run.ErrRuleIncomplete, err)
	}
	if err != nil {
		return run.Rules{}, err
	}
	return run.Rules{Set: rr.Rules, Tax: rr.Tax, Subsidy: rr.Subsidy}, nil
}

func TestRunCommit(t *testing.T) {
	seedCommit(t)
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	svc := run.NewService(app, ruleFunc, audit.NewWriter())
	ctx := asScopes(t, "SOLO")
	cmd := run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: commitYYMM, RuleSetID: 1}

	first, err := svc.Commit(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("commit %d karyawan: %s, %d baris, warnings %d", first.Summary.EmpCount, first.Duration, first.Summary.LineCount, len(first.Warnings))

	t.Run("commit", func(t *testing.T) {
		// Active SOLO employees seeded by earlier tests are in the run too (full suite).
		if first.Summary.EmpCount < commitEmps || first.SupersededID != 0 {
			t.Fatalf("summary %+v superseded %d", first.Summary, first.SupersededID)
		}
		if got := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_salary_head WHERE psh_run_id=%d AND psh_nik LIKE 'C%%'`, first.RunID)); got != fmt.Sprint(commitEmps) {
			t.Fatalf("head fixture = %s, want %d", got, commitEmps)
		}
		if first.Duration > 2*time.Minute {
			t.Fatalf("run 1 divisi %s > 2 menit (NF3)", first.Duration)
		}
		id := fmt.Sprint(first.RunID)
		if got := scalar(t, `SELECT pp_status||'/'||pp_active_run_id FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203203 AND pp_division='SOLO'`); got != "DRAFT/"+id {
			t.Fatalf("period = %s", got)
		}
		if got := scalar(t, `SELECT pr_status||'/'||pr_kind||'/'||pr_emp_count||'/'||pr_line_count||'/'||(pr_snapshot_id IS NOT NULL)||'/'||pr_started_by
			FROM payroll.pyr_run WHERE pr_id=`+id); got != fmt.Sprintf("ACTIVE/ENGINE/%d/%d/true/loader-test", first.Summary.EmpCount, first.Summary.LineCount) {
			t.Fatalf("run = %s", got)
		}
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_run_id=`+id); got != fmt.Sprint(first.Summary.LineCount) {
			t.Fatalf("detail rows %s, want %d", got, first.Summary.LineCount)
		}
		// Header totals = Σ detail; run totals = Σ header; employee attributes and sysid are joined.
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_head h WHERE h.psh_run_id=`+id+` AND h.psh_tot_tunjangan <>
			(SELECT COALESCE(sum(psd_amount),0) FROM payroll.pyr_salary_detail d WHERE d.psd_run_id=h.psh_run_id AND d.psd_nik=h.psh_nik AND d.psd_type='Tunjangan')`); got != "0" {
			t.Fatalf("%s header tidak cocok dengan detail", got)
		}
		// Run totals = Σ detail (an employee without recap has lines but no header, D2).
		if got := scalar(t, `SELECT (r.pr_tot_tunjangan = COALESCE(sum(d.psd_amount) FILTER (WHERE d.psd_type='Tunjangan'),0)
			AND r.pr_tot_potongan = COALESCE(sum(d.psd_amount) FILTER (WHERE d.psd_type='Potongan'),0))::text
			FROM payroll.pyr_run r JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id WHERE r.pr_id=`+id+` GROUP BY r.pr_id`); got != "true" {
			t.Fatal("run totals != Σ detail")
		}
		if got := scalar(t, `SELECT psh_nama||'/'||psh_dept||'/'||psh_tax_code||'/'||psh_basic_pay||'/'||(psh_jam_lembur->>'lembur1')
			FROM payroll.pyr_salary_head WHERE psh_run_id=`+id+` AND psh_nik='C00001'`); got != "Karyawan 1/PROD/TK/3001000.00/3" {
			t.Fatalf("head C00001 = %s", got)
		}
		if got := scalar(t, `SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_run_id=`+id+` AND (psd_component_sysid IS NULL OR psd_description IS NULL)`); got != "0" {
			t.Fatalf("%s baris tanpa sysid/deskripsi", got)
		}
		if got := scalar(t, `SELECT pal_action||'/'||pal_entity_key||'/'||(pal_after->>'input_hash') FROM payroll.pyr_audit_log WHERE pal_entity='pyr_run' ORDER BY pal_id DESC LIMIT 1`); got != "RUN_COMMIT/"+id+"/"+first.InputHash {
			t.Fatalf("audit = %s", got)
		}
	})

	t.Run("re-run supersedes, same input_hash and lines", func(t *testing.T) {
		second, err := svc.Commit(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		if second.SupersededID != first.RunID || second.InputHash != first.InputHash {
			t.Fatalf("second %+v", second)
		}
		if got := scalar(t, fmt.Sprintf(`SELECT string_agg(pr_status::text, ',' ORDER BY pr_id) FROM payroll.pyr_run WHERE pr_id IN (%d,%d)`, first.RunID, second.RunID)); got != "SUPERSEDED,ACTIVE" {
			t.Fatalf("status = %s", got)
		}
		diff := scalar(t, fmt.Sprintf(`SELECT count(*) FROM (
			(SELECT psd_nik, psd_type, psd_id, psd_amount, psd_meta FROM payroll.pyr_salary_detail WHERE psd_run_id=%d
			 EXCEPT SELECT psd_nik, psd_type, psd_id, psd_amount, psd_meta FROM payroll.pyr_salary_detail WHERE psd_run_id=%d)
			UNION ALL
			(SELECT psd_nik, psd_type, psd_id, psd_amount, psd_meta FROM payroll.pyr_salary_detail WHERE psd_run_id=%d
			 EXCEPT SELECT psd_nik, psd_type, psd_id, psd_amount, psd_meta FROM payroll.pyr_salary_detail WHERE psd_run_id=%d)) x`,
			first.RunID, second.RunID, second.RunID, first.RunID))
		if diff != "0" {
			t.Fatalf("re-run beda %s baris", diff)
		}
	})

	t.Run("failure mid-commit rolls back, previous run stays ACTIVE", func(t *testing.T) {
		active := scalar(t, `SELECT pp_active_run_id FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203203 AND pp_division='SOLO'`)
		bad := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: func(in engine.Input) (engine.Result, error) {
			a := dec("1")
			return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{ // qty overflows numeric(10,4): detail INSERT fails
				{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Qty: dec("1000000000")}}}, nil
		}})
		if _, err := bad.Commit(ctx, cmd); err == nil {
			t.Fatal("commit should fail")
		}
		if got := scalar(t, `SELECT pp_status||'/'||pp_active_run_id||'/'||(SELECT pr_status FROM payroll.pyr_run WHERE pr_id=pp_active_run_id)
			FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203203 AND pp_division='SOLO'`); got != "DRAFT/"+active+"/ACTIVE" {
			t.Fatalf("after rollback: %s", got)
		}
	})
	t.Run("parallel run rejected by lock", func(t *testing.T) {
		hold, err := raw.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = hold.Rollback() }()
		if _, err := hold.Exec(`SELECT pg_advisory_xact_lock(hashtext('payroll.run'), hashtext('1:203203:SOLO'))`); err != nil {
			t.Fatal(err)
		}
		_, err = svc.Commit(ctx, cmd)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeRunInProgress {
			t.Fatalf("got %v, want RUN_IN_PROGRESS", err)
		}
		// Another period is not blocked by this lock.
		if _, err := svc.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 203204, RuleSetID: 1}); err != nil {
			t.Fatalf("periode lain ikut terkunci: %v", err)
		}
	})

	t.Run("APPROVED and CLOSED rejected by service and trigger", func(t *testing.T) {
		before := scalar(t, `SELECT count(*) FROM payroll.pyr_run`)
		for _, st := range []struct {
			status string
			code   apperr.Code
		}{{"APPROVED", apperr.CodePeriodApproved}, {"CLOSED", apperr.CodePeriodClosed}} {
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_period SET pp_status=$1 WHERE pp_company_id=1 AND pp_yymm=203203 AND pp_division='SOLO'`, st.status); err != nil {
				t.Fatal(err)
			}
			_, err := svc.Commit(ctx, cmd)
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != st.code {
				t.Fatalf("%s: got %v", st.status, err)
			}
		}
		if after := scalar(t, `SELECT count(*) FROM payroll.pyr_run`); after != before {
			t.Fatalf("run tersimpan walau ditolak: %s → %s", before, after)
		}
		// Second layer (04§9): a direct write to the CLOSED period's result tables fails in the DB.
		err := app.ScopedTx(ctx, func(tx *sql.Tx) error {
			_, e := tx.Exec(`UPDATE payroll.pyr_salary_detail SET psd_amount = psd_amount + 1
				WHERE psd_run_id = (SELECT pp_active_run_id FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203203 AND pp_division='SOLO')`)
			return e
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
			t.Fatalf("trigger: got %v, want P0001", err)
		}
	})

	t.Run("unusable rule set is RULE_INCOMPLETE", func(t *testing.T) {
		_, err := svc.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 203204, RuleSetID: 99})
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeRuleIncomplete {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("division outside scope", func(t *testing.T) {
		_, err := svc.Commit(asScopes(t, "JAKARTA"), cmd)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeDivisionForbidden {
			t.Fatalf("got %v", err)
		}
	})
}

// T017 + D-38: a P0 line with subsidy_basis_source = LOCK_NOW is stored with its meta, so the
// v2 loader reads it back as the lock of the following periods.
func TestRunCommit_SubsidyLockMeta(t *testing.T) {
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	ctx := asScopes(t, "SOLO")
	// Persist through the service with a stub calculation that emits a LOCK_NOW P0 line.
	stub := func(in engine.Input) (engine.Result, error) {
		a, z := dec("1000"), dec("0")
		return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{
			{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true},
			{Key: engine.CompKey{Type: engine.CompP, ID: 0}, Amount: &z, Meta: map[string]string{
				engine.MetaSubsidySource: engine.SubsidySrcLockNow, engine.MetaSubsidyBasis: "3001000"}},
		}, Header: &engine.Header{TotTunjangan: a}}, nil
	}
	svc := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: stub})
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203205,'SOLO',26), (1,203206,'SOLO',26);
		INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		VALUES (1,203205,'SOLO','{1}','lock',true), (1,203206,'SOLO','{1}','lock',true);
		INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		SELECT s.pas_snapshot_id, 'REKAP', 'C00001', '{"MPAR_DAY_WORK":26}'::jsonb FROM payroll.pyr_att_snapshot s WHERE s.pas_checksum = 'lock'`); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 203205, RuleSetID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, fmt.Sprintf(`SELECT psd_meta->>'subsidy_basis_source'||'/'||(psd_meta->>'subsidy_basis') FROM payroll.pyr_salary_detail
		WHERE psd_run_id=%d AND psd_nik='C00001' AND psd_type='Potongan' AND psd_id=0`, res.RunID)); got != "LOCK_NOW/3001000" {
		t.Fatalf("meta = %s", got)
	}
	var locked engine.Subsidy
	err = app.ScopedTx(ctx, func(tx *sql.Tx) error {
		r, e := run.NewLoader().Load(context.Background(), tx, run.Query{CompanyID: 1, Division: "SOLO", YYMM: 203206, NIKs: []string{"C00001"}},
			run.Rules{Set: stubRules{false}})
		if e == nil && len(r.Inputs) == 1 {
			locked = r.Inputs[0].Subsidy
		}
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if locked.Locked == nil || !locked.Locked.Equal(dec("3001000")) || locked.LockedYYMM != 203205 {
		t.Fatalf("lock v2 = %v @%d", locked.Locked, locked.LockedYYMM)
	}
}
