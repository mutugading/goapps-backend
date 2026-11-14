package etl

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// waitWaiters polls pg_locks until n lock requests are queued (no fixed sleep as synchronization).
func waitWaiters(t *testing.T, n string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for scalar(t, `SELECT (count(*) >= `+n+`)::text FROM pg_locks WHERE NOT granted AND locktype='advisory'`) != "true" {
		select {
		case err := <-done:
			t.Fatalf("selesai sebelum menunggu lock: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("lock waiter < %s", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

const concSnapWhere = `WHERE pas_company_id=1 AND pas_yymm=202607 AND pas_division='SOLO' AND pas_source_batch_ids='{}'`

// concSetup prepares 202607 SOLO for an ETL-vs-Commit race (T017f/T017g, D-40): a legacy snapshot made inert
// ('keep', unlocked) with one numeric REKAP row for C00001 plus one for 2026-0001 (sentinel fail columns = 7, the
// nik testdata_fail writes to). It returns a run service whose engine stub parks inside Commit until release is
// closed (safe to call more than once via the returned func), and a channel closed when the stub is reached.
func concSetup(t *testing.T) (runs *run.Service, started <-chan struct{}, release func()) {
	t.Helper()
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.AttLegacyOptions{CompanyID: 1}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	if scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_status='ACTIVE'`) != "0" {
		t.Fatal("fixture berubah: 202607 SOLO sudah punya run ACTIVE")
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`UPDATE payroll.pyr_period SET pp_active_run_id=NULL WHERE pp_company_id=1 AND pp_yymm=202607 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id IN (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot ` + concSnapWhere + `)`,
			`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO'`,
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202607 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_attd_days WHERE pad_company_id=1 AND pad_yymm=202607 AND pad_division='SOLO'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
			t.Errorf("cleanup rerun: %v", err)
		}
	})
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot SET pas_checksum='keep', pas_locked=false ` + concSnapWhere); err != nil {
		t.Fatal(err)
	}
	// The loader reads numbers only; legacy REKAP rows carry text (MPAR_NAMA). Swap in numeric rows;
	// the cleanup ETL rerun restores the real ones.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,202607,'SOLO',26) ON CONFLICT DO NOTHING`,
		`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id IN (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot ` + concSnapWhere + `)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data, pasr_premi_fail, pasr_beras_fail)
		 SELECT pas_snapshot_id, 'REKAP', n, '{"MPAR_DAY_WORK":26}'::jsonb, 7, 7 FROM payroll.pyr_att_snapshot, (VALUES ('C00001'),('2026-0001')) v(n) ` + concSnapWhere,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	st, rel := make(chan struct{}), make(chan struct{})
	var once, relOnce sync.Once
	release = func() { relOnce.Do(func() { close(rel) }) }
	t.Cleanup(release) // never leave the Commit goroutine parked (would hang Cleanup/DB close)
	gated := func(in engine.Input) (engine.Result, error) {
		once.Do(func() { close(st); <-rel })
		a := dec("1000")
		return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
			Header: &engine.Header{TotTunjangan: a}}, nil
	}
	return run.NewServiceWith(postgres.NewDBFromSQL(raw), ruleFunc, audit.NewWriter(), run.Runner{Calc: gated}), st, release
}

// concRace parks Commit in the engine, starts etlFn, waits until it queues on the run advisory lock (pg_locks),
// then releases Commit and waits for both.
func concRace(t *testing.T, etlFn func() error) {
	t.Helper()
	runs, started, release := concSetup(t)
	commitDone := make(chan error, 1)
	go func() {
		_, e := runs.Commit(asScopes(t, "SOLO"), run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: 202607, RuleSetID: 1})
		commitDone <- e
	}()
	select {
	case <-started:
	case err := <-commitDone:
		t.Fatalf("commit selesai sebelum engine: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("commit tidak mencapai engine")
	}
	etlDone := make(chan error, 1)
	go func() { etlDone <- etlFn() }()
	waitWaiters(t, "1", etlDone) // ETL queued behind the run advisory lock
	release()
	if err := <-commitDone; err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-etlDone; err != nil {
		t.Fatalf("etl: %v", err)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_status='ACTIVE' AND pr_kind='ENGINE'`); got != "1" {
		t.Fatalf("run ACTIVE = %s, want 1", got)
	}
}

// T017f (D-40): RunAttLegacy waits behind a running run.Commit; once Commit binds an ENGINE ACTIVE run the
// snapshot must stay untouched and the period is reported in Skipped.
func TestETL_AttLegacyVsCommitConcurrent(t *testing.T) {
	var reps []etl.TableReport
	concRace(t, func() (err error) {
		reps, err = etl.RunAttLegacy(context.Background(), etlDB, etl.PSVDir{Dir: "testdata"}, etl.AttLegacyOptions{CompanyID: 1})
		return err
	})
	if got := scalar(t, `SELECT pas_checksum||'/'||pas_locked::text FROM payroll.pyr_att_snapshot `+concSnapWhere); got != "keep/false" {
		t.Fatalf("snapshot berubah setelah Commit mengikat run ACTIVE: %s", got)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id `+
		strings.ReplaceAll(concSnapWhere, "pas_", "s.pas_")); got != "2" {
		t.Fatalf("baris snapshot berubah oleh ETL: %s", got)
	}
	found := false
	for _, r := range reps {
		for _, s := range r.Skipped {
			found = found || (strings.Contains(s, "202607/SOLO") && strings.Contains(s, "run ENGINE ACTIVE"))
		}
	}
	if !found {
		t.Fatalf("Skipped tidak memuat 202607/SOLO run ENGINE ACTIVE: %+v", reps)
	}
}

// T017g (D-40): RunAttLegacyFail takes the same run lock before its UPDATE, so after a concurrent Commit it must
// not touch the REKAP rows (sentinel 7 stays).
func TestETL_AttLegacyFailVsCommitConcurrent(t *testing.T) {
	concRace(t, func() error {
		_, err := etl.RunAttLegacyFail(context.Background(), etlDB, etl.PSVDir{Dir: "testdata_fail"}, etl.AttLegacyFailOptions{})
		return err
	})
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id `+
		strings.ReplaceAll(concSnapWhere, "pas_", "s.pas_")+` AND (r.pasr_premi_fail IS DISTINCT FROM 7 OR r.pasr_beras_fail IS DISTINCT FROM 7)`); got != "0" {
		t.Fatalf("%s baris REKAP diubah ETL fail setelah Commit mengikat run ACTIVE", got)
	}
}
