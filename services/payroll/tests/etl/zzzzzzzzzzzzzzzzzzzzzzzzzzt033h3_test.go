package etl

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T033h3: Salary/THR Payables rows differing only in MPS_DEPT share one key (the legacy join ignores dept): identical
// accounts collapse, different accounts are a conflict (exception), never an arbitrary pick.
func TestT033h3_GlPayableIgnoresDept(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_gl_setup WHERE pgs_company_id=1 AND pgs_gl_group LIKE 'T3H3-%'`); err != nil {
		t.Fatal(err)
	}
	dir := writePSV(t, "MGT_PAYROLL_SETUP", glHdr,
		glRow("T3H3-Same", "", "Salary Payables", "MGT", "205012", "205012", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("T3H3-Same", "", "Salary Payables", "MB", "205012", "205012", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("T3H3-Diff", "", "Salary Payables", "MGT", "205012", "205012", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("T3H3-Diff", "", "Salary Payables", "MB", "205099", "205099", "", "C", "Salary Payables", "GAJI", "SOLO"),
		glRow("T3H3-Odd", "", "Salary Payables", "XX", "205011", "205011", "", "C", "Dept aneh diabaikan", "GAJI", "SOLO"),
	)
	rep, err := etl.RunGlSetup(ctx, etlDB, dir, etl.GlSetupOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SrcRows != 5 || rep.DstRows != 3 || len(rep.Exceptions) != 2 || !rep.OK() {
		t.Fatalf("src=%d dst=%d exc=%v", rep.SrcRows, rep.DstRows, rep.Exceptions)
	}
	if got := scalar(t, `SELECT string_agg(pgs_gl_group||':'||coalesce(pgs_cost_group,'-')||':'||pgs_account_main, ',' ORDER BY 1) FROM payroll.pyr_gl_setup
		WHERE pgs_company_id=1 AND pgs_gl_group LIKE 'T3H3-%'`); got != "T3H3-Odd:-:205011,T3H3-Same:-:205012" {
		t.Fatalf("PAYABLE = %q, mau hanya T3H3-Same tanpa cost group", got)
	}
	// A PAYABLE row with a cost group (loaded before T033h3) is reported, not used.
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, pgs_cost_group,
		pgs_line_kind, pgs_account_main, pgs_drcr, pgs_description, pgs_valid_from, pgs_legacy_row)
		VALUES (1,'SOLO','GAJI','T3H3-Same','MB','PAYABLE','205077','C','lama',201712,'{}')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_gl_setup WHERE pgs_company_id=1 AND pgs_gl_group LIKE 'T3H3-%'`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	rep, err = etl.RunGlSetup(ctx, etlDB, dir, etl.GlSetupOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), `T3H3-Same/cost="MB"`) {
		t.Errorf("baris PAYABLE lama tidak dilaporkan: %v", rep.Warnings)
	}
}

// T033h3: the legacy rule constants report counts what the statements wrote (RowsAffected), not len(list).
func TestT033h3_RuleConstantsCounted(t *testing.T) {
	ctx := context.Background()
	tx, err := etlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	rep, err := etl.LoadRuleConstants(ctx, tx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SrcRows == 0 || rep.DstRows != rep.SrcRows || !rep.OK() {
		t.Fatalf("src=%d dst=%d", rep.SrcRows, rep.DstRows)
	}
}

// T033h3: a division without a daily row on the 25th passes the window gate when its newest OK daily batch started
// after the window closed (26th 00:00 WIB); a batch started inside the window is still refused.
func TestT033h3_DailyWindowGateByBatchTime(t *testing.T) {
	const ym = 203711
	part := []etl.DailyRow{{WorkDate: time.Date(2037, 11, 20, 0, 0, 0, 0, time.UTC), NIK: "SN-H3", Division: "SOLO", ScheduleCode: "NW"}}
	syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "SN-H3", "SOLO", "20")}, daily: part}, ym, "SOLO")
	// Batches run "now" (2026): move the daily batch inside the window first.
	setStart := func(ts string) {
		t.Helper()
		if _, err := ownDB.Exec(fmt.Sprintf(`UPDATE payroll_stg.pyr_stg_sync_batch SET psb_started_at = '%s'
			WHERE psb_company_id=1 AND psb_yymm=%d AND psb_division='SOLO' AND psb_source='MGTHRIS.HM_ATT_DAILY'`, ts, ym)); err != nil {
			t.Fatal(err)
		}
	}
	snap := func(allow bool) ([]etl.AttSnapshotReport, error) {
		return etl.RunAttSnapshot(context.Background(), etlDB, etl.AttSnapshotQuery{Company: 1, YYMM: ym, Division: "SOLO", AllowNoDay25: allow})
	}
	setStart("2037-11-26 00:00:00+07")
	if reps, err := snap(false); err == nil || !strings.Contains(err.Error(), "allow-no-day25") || len(reps) != 0 {
		t.Fatalf("tanpa override: mau ditolak, dapat %v %+v", err, reps)
	}
	setStart("2037-11-25 23:59:00+07")
	if reps, err := snap(true); err == nil || !strings.Contains(err.Error(), "dimulai sebelum jendela selesai") || len(reps) != 0 {
		t.Fatalf("override, batch di dalam jendela: mau ditolak, dapat %v %+v", err, reps)
	}
	setStart("2037-11-26 00:00:00+07")
	if reps, err := snap(true); err != nil || len(reps) != 1 || reps[0].SnapshotID == 0 {
		t.Fatalf("override, batch sesudah jendela: %v %+v", err, reps)
	}
}

// T033h3: snapshot and dummy take the run lock instead of FOR SHARE. A period transition holding that lock (as
// approve/close do via run.Lock) that commits CLOSED must make the queued ETL refuse, not write into a CLOSED period.
func TestT033h3_SnapshotAndDummyWaitForClose(t *testing.T) {
	ctx := context.Background()
	const ym = 203712
	full := []etl.DailyRow{{WorkDate: time.Date(2037, 12, 25, 0, 0, 0, 0, time.UTC), NIK: "SN-H3", Division: "SOLO", ScheduleCode: "NW"}}
	syncAll(t, &fakeAtt{rekap: []etl.RekapRow{rrow(ym, "SN-H3", "SOLO", "20")}, daily: full}, ym, "SOLO")
	if _, err := ownDB.Exec(fmt.Sprintf(`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		VALUES (1,%d,'SOLO','2037-11-26','2037-12-25','OPEN')`, ym)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(etl.DummyDBAllowEnv, "payroll_db")
	t.Setenv("APP_ENV", "staging")
	for name, fn := range map[string]func() error{
		"snapshot": func() error {
			_, err := etl.RunAttSnapshot(ctx, etlDB, etl.AttSnapshotQuery{Company: 1, YYMM: ym, Division: "SOLO"})
			return err
		},
		"dummy": func() error {
			seedDummySource(t)
			_, err := etl.RunDummyPeriod(ctx, etlDB, etl.DummyPeriodQuery{Company: 1, From: 204010, To: ym, Division: "SOLO"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ownDB.Exec(fmt.Sprintf(`UPDATE payroll.pyr_period SET pp_status='OPEN' WHERE pp_company_id=1 AND pp_yymm=%d AND pp_division='SOLO'`, ym)); err != nil {
				t.Fatal(err)
			}
			n0 := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=%d`, ym))
			holder, err := ownDB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback() }()
			if _, err := holder.ExecContext(ctx, fmt.Sprintf(`SELECT pg_advisory_xact_lock(hashtext('payroll.run'), hashtext('1:%d:SOLO'))`, ym)); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- fn() }()
			waitWaiters(t, fmt.Sprintf("1:%d:SOLO", ym), "1", done) // ETL queued behind the run lock
			if _, err := holder.ExecContext(ctx, fmt.Sprintf(`UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_company_id=1 AND pp_yymm=%d AND pp_division='SOLO'`, ym)); err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if err == nil || !strings.Contains(err.Error(), "CLOSED") {
				t.Fatalf("ETL sesudah close: mau ditolak CLOSED, dapat %v", err)
			}
			if n := scalar(t, fmt.Sprintf(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_yymm=%d`, ym)); n != n0 {
				t.Fatalf("snapshot ditulis ke periode CLOSED (%s -> %s)", n0, n)
			}
		})
	}
}
