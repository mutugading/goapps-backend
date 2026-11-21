package etl

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/period"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T017b: approve → close → reopen of period 203207 SOLO against PG18 (employee C00001 from seedCommit,
// TestRunCommit runs earlier in file order). The snapshot starts unlocked: the loader reads it anyway (D-40).
func TestPeriodTransitions(t *testing.T) {
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	app := postgres.NewDBFromSQL(raw)
	ctx := asScopes(t, "SOLO")
	stub := func(in engine.Input) (engine.Result, error) {
		a := dec("1000")
		return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
			Header: &engine.Header{TotTunjangan: a}}, nil
	}
	runs := run.NewServiceWith(app, ruleFunc, audit.NewWriter(), run.Runner{Calc: stub})
	svc := period.NewService(app, audit.NewWriter(), runs)
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203207,'SOLO',26);
		INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		VALUES (1,203207,'SOLO','{1}','period',false);
		INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		SELECT s.pas_snapshot_id, 'REKAP', 'C00001', '{"MPAR_DAY_WORK":26}'::jsonb FROM payroll.pyr_att_snapshot s WHERE s.pas_checksum = 'period'`); err != nil {
		t.Fatal(err)
	}
	commit := run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 203207, RuleSetID: 1}
	first, err := runs.Commit(ctx, commit)
	if err != nil {
		t.Fatalf("commit pada snapshot belum terkunci: %v", err)
	}
	cmd := period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203207, IP: "10.0.0.9"}
	const where = ` FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203207 AND pp_division='SOLO'`
	snapLocked := func() string {
		return scalar(t, `SELECT pas_locked::text FROM payroll.pyr_att_snapshot WHERE pas_checksum='period'`)
	}
	guard := func() error {
		return app.ScopedTx(ctx, func(tx *sql.Tx) error { return period.Guard(ctx, tx, 1, 203207, "SOLO") })
	}
	wantCode := func(t *testing.T, err error, code apperr.Code) {
		t.Helper()
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != code {
			t.Fatalf("got %v, want %s", err, code)
		}
	}
	lastAudit := func() string {
		return scalar(t, `SELECT pal_action||'/'||pal_actor||'/'||pal_entity_key||'/'||(pal_before->>'status')||'/'||(pal_after->>'status')
			FROM payroll.pyr_audit_log WHERE pal_entity='pyr_period' ORDER BY pal_id DESC LIMIT 1`)
	}

	t.Run("DRAFT: close and reopen refused, guard open", func(t *testing.T) {
		wantCode(t, svc.Close(ctx, cmd), apperr.CodeConflict)
		wantCode(t, svc.Reopen(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203207, Reason: "salah input absensi"}), apperr.CodeConflict)
		if err := guard(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("approve locks the snapshot", func(t *testing.T) {
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		if got := scalar(t, `SELECT pp_status||'/'||approved_by||'/'||(approved_at IS NOT NULL)`+where); got != "APPROVED/loader-test/true" {
			t.Fatalf("period = %s", got)
		}
		if snapLocked() != "true" {
			t.Fatal("snapshot tidak terkunci")
		}
		if got := lastAudit(); got != "PERIOD_APPROVE/loader-test/203207:SOLO/DRAFT/APPROVED" {
			t.Fatalf("audit = %s", got)
		}
		wantCode(t, guard(), apperr.CodePeriodApproved)
		_, err := runs.Commit(ctx, commit)
		wantCode(t, err, apperr.CodePeriodApproved)
		wantCode(t, svc.Approve(ctx, cmd), apperr.CodePeriodApproved)
	})
	t.Run("close: service and trigger reject mutations (I24)", func(t *testing.T) {
		if err := svc.Close(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		if got := scalar(t, `SELECT pp_status||'/'||closed_by`+where); got != "CLOSED/loader-test" {
			t.Fatalf("period = %s", got)
		}
		wantCode(t, guard(), apperr.CodePeriodClosed)
		for _, q := range []string{
			`INSERT INTO payroll.pyr_addition (pa_company_id, pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount) VALUES (1,203207,'C00001','SOLO','Tunjangan',5,1)`,
			fmt.Sprintf(`UPDATE payroll.pyr_salary_detail SET psd_amount = psd_amount + 1 WHERE psd_run_id = %d`, first.RunID),
			fmt.Sprintf(`DELETE FROM payroll.pyr_salary_head WHERE psh_run_id = %d`, first.RunID),
		} {
			err := app.ScopedTx(ctx, func(tx *sql.Tx) error { _, e := tx.Exec(q); return e })
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
				t.Fatalf("%s: got %v, want P0001", q, err)
			}
		}
		// D-54 (000038): input tables read by the loader are guarded too, even for the schema owner.
		for _, q := range []string{
			`INSERT INTO payroll.pyr_tax_adj (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type, pta_amount) VALUES (1,203207,'SOLO','C00001','T',1)`,
			`UPDATE payroll.pyr_attd_days SET pad_days = 25 WHERE pad_company_id=1 AND pad_yymm=203207 AND pad_division='SOLO'`,
			`UPDATE payroll.pyr_att_snapshot SET pas_checksum = 'x' WHERE pas_checksum='period'`,
			`UPDATE payroll.pyr_att_snapshot_row SET pasr_data = '{}' WHERE pasr_snapshot_id = (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_checksum='period')`,
		} {
			_, err := ownDB.Exec(q)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
				t.Fatalf("%s: got %v, want P0001", q, err)
			}
		}
	})
	t.Run("reopen needs a reason of 10+ characters", func(t *testing.T) {
		wantCode(t, svc.Reopen(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203207, Reason: "  singkat  "}), apperr.CodeValidation)
		if got := scalar(t, `SELECT pp_status::text`+where); got != "CLOSED" {
			t.Fatalf("status = %s", got)
		}
	})
	t.Run("reopen unlocks, re-run supersedes", func(t *testing.T) {
		re := cmd
		re.Reason = "koreksi lembur 3 karyawan"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
		if got := scalar(t, `SELECT pp_status||'/'||pp_reopen_reason||'/'||(approved_by IS NULL AND approved_at IS NULL AND closed_by IS NULL AND closed_at IS NULL)||'/'||pp_active_run_id`+where); got != fmt.Sprintf("DRAFT/koreksi lembur 3 karyawan/true/%d", first.RunID) {
			t.Fatalf("period = %s", got)
		}
		if snapLocked() != "false" {
			t.Fatal("lock snapshot tidak dibuka")
		}
		if got := lastAudit(); got != "PERIOD_REOPEN/loader-test/203207:SOLO/CLOSED/DRAFT" {
			t.Fatalf("audit = %s", got)
		}
		second, err := runs.Commit(ctx, commit)
		if err != nil || second.SupersededID != first.RunID {
			t.Fatalf("re-run setelah reopen: %+v %v", second, err)
		}
	})
	t.Run("approve needs an ACTIVE run, unknown period, scope", func(t *testing.T) {
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			VALUES (1,203208,'SOLO','2032-07-26','2032-08-25','DRAFT')`); err != nil {
			t.Fatal(err)
		}
		wantCode(t, svc.Approve(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203208}), apperr.CodeBusinessRule)
		wantCode(t, svc.Approve(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 209901}), apperr.CodeNotFound)
		wantCode(t, svc.Approve(asScopes(t, "JAKARTA"), cmd), apperr.CodeDivisionForbidden)
	})
	t.Run("approve clears reopen reason; reopen from APPROVED", func(t *testing.T) {
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		re := cmd
		re.Reason = "salah hitung lembur"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatalf("reopen dari APPROVED: %v", err)
		}
		if got := scalar(t, `SELECT pp_status||'/'||pp_reopen_reason`+where); got != "DRAFT/salah hitung lembur" {
			t.Fatalf("period = %s", got)
		}
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		if got := scalar(t, `SELECT pp_status||'/'||(pp_reopen_reason IS NULL)`+where); got != "APPROVED/true" {
			t.Fatalf("alasan reopen tidak dikosongkan: %s", got)
		}
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("D-54: stale input refuses approve and close", func(t *testing.T) {
		days := func(n int) {
			t.Helper()
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_attd_days SET pad_days=$1 WHERE pad_company_id=1 AND pad_yymm=203207 AND pad_division='SOLO'`, n); err != nil {
				t.Fatal(err)
			}
		}
		days(25)
		wantCode(t, svc.Approve(ctx, cmd), apperr.CodeConflict)
		if got := scalar(t, `SELECT pp_status::text`+where); got != "DRAFT" {
			t.Fatalf("status = %s", got)
		}
		days(26)
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatalf("input dikembalikan, approve harus lolos: %v", err)
		}
		days(25) // APPROVED has no DB layer (D-40): period.Guard is the service layer, close re-checks the input
		wantCode(t, svc.Close(ctx, cmd), apperr.CodeConflict)
		days(26)
		re := cmd
		re.Reason = "kembali ke draft untuk tes"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("D-54: blocking warnings refuse approve, NO_WORKDAYS does not", func(t *testing.T) {
		setWarn := func(js string) {
			t.Helper()
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_run SET pr_warnings=$1::jsonb WHERE pr_id=(SELECT pp_active_run_id`+where+`)`, js); err != nil {
				t.Fatal(err)
			}
		}
		defer setWarn("[]")
		for _, code := range []string{run.CodeCalcError, run.CodeLoanDouble, run.CodeIncomplete} {
			setWarn(`[{"nik":"C00009","code":"` + code + `"},{"nik":"C00009","code":"ENGINE_WARNING"}]`)
			err := svc.Approve(ctx, cmd)
			wantCode(t, err, apperr.CodeBusinessRule)
			var ae *apperr.Error
			if errors.As(err, &ae) && (ae.Details["niks"] != "C00009" || ae.Details["count"] != "1") {
				t.Fatalf("%s: detail = %v", code, ae.Details)
			}
		}
		setWarn(`[{"nik":"C00009","code":"NO_WORKDAYS"},{"code":"SUBSIDY_LOCK_STALE"}]`)
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatalf("NO_WORKDAYS tidak memblokir: %v", err)
		}
		re := cmd
		re.Reason = "kembali ke draft untuk tes"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("D-54: blocking warning without NIK still blocks", func(t *testing.T) {
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_run SET pr_warnings='[{"code":"CALC_ERROR"}]'::jsonb WHERE pr_id=(SELECT pp_active_run_id` + where + `)`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_run SET pr_warnings='[]'::jsonb WHERE pr_id=(SELECT pp_active_run_id` + where + `)`); err != nil {
				t.Fatal(err)
			}
		}()
		err := svc.Approve(ctx, cmd)
		wantCode(t, err, apperr.CodeBusinessRule)
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Details["count"] != "1" {
			t.Fatalf("detail = %v", ae.Details)
		}
	})
	t.Run("D-54: changed snapshot row after the run refuses approve", func(t *testing.T) {
		const snapRow = `UPDATE payroll.pyr_att_snapshot_row SET pasr_data = '{"MPAR_DAY_WORK":%d}'::jsonb
WHERE pasr_snapshot_id = (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_checksum='period') AND pasr_nik='C00001'`
		if _, err := ownDB.Exec(fmt.Sprintf(snapRow, 20)); err != nil {
			t.Fatal(err)
		}
		wantCode(t, svc.Approve(ctx, cmd), apperr.CodeConflict)
		if _, err := ownDB.Exec(fmt.Sprintf(snapRow, 26)); err != nil {
			t.Fatal(err)
		}
		if got := scalar(t, `SELECT pp_status::text`+where); got != "DRAFT" {
			t.Fatalf("status = %s", got)
		}
	})
	t.Run("D-54: new tax_adj after a December run refuses approve", func(t *testing.T) {
		// pyr_tax_adj is read by the loader only for December periods, so this case uses its own period 203312.
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203312,'SOLO',26);
			INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
			VALUES (1,203312,'SOLO','{1}','period-dec2',false);
			INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
			SELECT s.pas_snapshot_id, 'REKAP', 'C00001', '{"MPAR_DAY_WORK":26}'::jsonb FROM payroll.pyr_att_snapshot s WHERE s.pas_checksum = 'period-dec2'`); err != nil {
			t.Fatal(err)
		}
		// zzztax leaves SOLO employees whose NPWP chain conflicts (Y-C, Y-E); any December SOLO run would fail on
		// them (13§3.3). Cut their chain for this subtest and restore it afterwards.
		const chain = `SELECT pe_nik, pe_prev_employee_id FROM payroll.pyr_employee WHERE pe_company_id=1 AND pe_nik IN ('Y-C','Y-E') AND pe_prev_employee_id IS NOT NULL`
		saved := map[string]int64{}
		rows, err := ownDB.Query(chain)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var nik string
			var prev int64
			if err := rows.Scan(&nik, &prev); err != nil {
				t.Fatal(err)
			}
			saved[nik] = prev
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_prev_employee_id=NULL WHERE pe_company_id=1 AND pe_nik IN ('Y-C','Y-E')`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			for nik, prev := range saved {
				if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_prev_employee_id=$1 WHERE pe_company_id=1 AND pe_nik=$2`, prev, nik); err != nil {
					t.Fatal(err)
				}
			}
		}()
		dec := period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203312}
		if _, err := runs.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 203312, RuleSetID: 1}); err != nil {
			t.Fatalf("commit periode Desember: %v", err)
		}
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_tax_adj (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type, pta_amount) VALUES (1,203312,'SOLO','C00001','Tunjangan',5)`); err != nil {
			t.Fatal(err)
		}
		wantCode(t, svc.Approve(ctx, dec), apperr.CodeConflict)
		if got := scalar(t, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_yymm=203312 AND pp_division='SOLO'`); got != "DRAFT" {
			t.Fatalf("status = %s", got)
		}
	})
	t.Run("D-54: ENGINE run without input hash is an internal error", func(t *testing.T) {
		const act = `(SELECT pp_active_run_id` + where + `)`
		orig := scalar(t, `SELECT pr_input_hash FROM payroll.pyr_run WHERE pr_id=`+act)
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_run SET pr_input_hash=NULL WHERE pr_id=` + act); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := ownDB.Exec(`UPDATE payroll.pyr_run SET pr_input_hash=$1 WHERE pr_id=`+act, orig); err != nil {
				t.Fatal(err)
			}
		}()
		err := svc.Approve(ctx, cmd)
		var ae *apperr.Error
		if err == nil || errors.As(err, &ae) {
			t.Fatalf("want plain internal error, got %v", err)
		}
		if got := scalar(t, `SELECT pp_status::text`+where); got != "DRAFT" {
			t.Fatalf("status = %s", got)
		}
	})
	t.Run("LEGACY run: reopen refused", func(t *testing.T) {
		for _, q := range []string{
			`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
			 VALUES (1,203209,'SOLO','2032-08-26','2032-09-25','OPEN')`,
			`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_status, pr_kind)
			 VALUES (1,203209,'SOLO','2032-09-25',0,'ACTIVE','LEGACY')`,
			`UPDATE payroll.pyr_period SET pp_status='CLOSED', closed_by='etl',
			   pp_active_run_id=(SELECT pr_id FROM payroll.pyr_run WHERE pr_yymm=203209 AND pr_division='SOLO')
			 WHERE pp_company_id=1 AND pp_yymm=203209 AND pp_division='SOLO'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatalf("%v\n%s", err, q)
			}
		}
		wantCode(t, svc.Reopen(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203209, Reason: "koreksi data lama"}), apperr.CodeBusinessRule)
		if got := scalar(t, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_yymm=203209 AND pp_division='SOLO'`); got != "CLOSED" {
			t.Fatalf("status = %s", got)
		}
	})
	t.Run("LEGACY run not active: reopen still refused", func(t *testing.T) {
		for _, q := range []string{
			`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status, closed_by)
			 VALUES (1,203210,'SOLO','2032-09-26','2032-10-25','CLOSED','etl')`,
			`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_status, pr_kind)
			 VALUES (1,203210,'SOLO','2032-10-25',0,'SUPERSEDED','LEGACY')`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Fatalf("%v\n%s", err, q)
			}
		}
		wantCode(t, svc.Reopen(ctx, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203210, Reason: "koreksi data lama"}), apperr.CodeBusinessRule)
	})
	t.Run("I24: JAKARTA user vs SOLO CLOSED period", func(t *testing.T) {
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_period SET pp_status='DRAFT', closed_by=NULL, closed_at=NULL, approved_by=NULL, approved_at=NULL
			WHERE pp_company_id=1 AND pp_yymm=203207 AND pp_division='SOLO'`); err != nil {
			t.Fatal(err)
		}
		if err := svc.Approve(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		if err := svc.Close(ctx, cmd); err != nil {
			t.Fatal(err)
		}
		jkt := asScopes(t, "JAKARTA")
		wantCode(t, svc.Reopen(jkt, period.Command{CompanyID: 1, Division: "SOLO", YYMM: 203207, Reason: "bukan divisi saya"}), apperr.CodeDivisionForbidden)
		wantCode(t, svc.Close(jkt, cmd), apperr.CodeDivisionForbidden)
		// SOLO CLOSED does not block JAKARTA of the same yymm (no JAKARTA period = OPEN).
		if err := app.ScopedTx(jkt, func(tx *sql.Tx) error { return period.Guard(jkt, tx, 1, 203207, "JAKARTA") }); err != nil {
			t.Fatalf("JAKARTA terblokir oleh SOLO CLOSED: %v", err)
		}
		if got := scalar(t, `SELECT pp_status::text`+where); got != "CLOSED" {
			t.Fatalf("status SOLO = %s", got)
		}
		re := cmd
		re.Reason = "buka lagi untuk tes"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("Guard waits for a concurrent approve and sees APPROVED", func(t *testing.T) {
		tx, err := ownDB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after commit
		if _, err := tx.Exec(`SELECT 1` + where + ` FOR UPDATE`); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- guard() }()
		// Guard is blocked once a lock request is queued in pg_locks (no fixed sleep).
		deadline := time.Now().Add(10 * time.Second)
		for {
			select {
			case err := <-done:
				t.Fatalf("Guard tidak menunggu row lock: %v", err)
			default:
			}
			if scalar(t, `SELECT count(*) FROM pg_locks WHERE NOT granted`) != "0" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Guard tidak pernah terlihat menunggu lock")
			}
			time.Sleep(20 * time.Millisecond)
		}
		if _, err := tx.Exec(`UPDATE payroll.pyr_period SET pp_status='APPROVED'` + where[len(` FROM payroll.pyr_period`):]); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		wantCode(t, <-done, apperr.CodePeriodApproved)
		re := cmd
		re.Reason = "pulihkan setelah tes konkuren"
		if err := svc.Reopen(ctx, re); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("run lock held: transition fails fast", func(t *testing.T) {
		hold, err := raw.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = hold.Rollback() }()
		if _, err := hold.Exec(`SELECT pg_advisory_xact_lock(hashtext('payroll.run'), hashtext('1:203207:SOLO'))`); err != nil {
			t.Fatal(err)
		}
		wantCode(t, svc.Approve(ctx, cmd), apperr.CodeRunInProgress)
	})
}
