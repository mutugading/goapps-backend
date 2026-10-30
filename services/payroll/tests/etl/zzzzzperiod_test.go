package etl

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

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
	svc := period.NewService(app, audit.NewWriter())
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
