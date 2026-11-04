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
	for scalar(t, `SELECT (count(*) >= `+n+`)::text FROM pg_locks WHERE NOT granted`) != "true" {
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

// T017f (D-40): RunAttLegacy waits (pg_locks) behind a running run.Commit that binds an ENGINE ACTIVE run;
// afterwards the snapshot must stay untouched once Commit binds
// an ENGINE ACTIVE run.
func TestETL_AttLegacyVsCommitConcurrent(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.AttLegacyOptions{CompanyID: 1}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	const snapWhere = `WHERE pas_company_id=1 AND pas_yymm=202607 AND pas_division='SOLO' AND pas_source_batch_ids='{}'`
	if scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_status='ACTIVE'`) != "0" {
		t.Fatal("fixture berubah: 202607 SOLO sudah punya run ACTIVE")
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`UPDATE payroll.pyr_period SET pp_active_run_id=NULL WHERE pp_company_id=1 AND pp_yymm=202607 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id IN (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot WHERE pas_company_id=1 AND pas_yymm=202607 AND pas_division='SOLO' AND pas_source_batch_ids='{}')`,
			`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO'`,
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202607 AND pp_division='SOLO'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
			t.Errorf("cleanup rerun: %v", err)
		}
	})
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_att_snapshot SET pas_checksum='keep', pas_locked=false ` + snapWhere); err != nil {
		t.Fatal(err)
	}

	// The loader reads numbers only; legacy REKAP rows carry text (MPAR_NAMA). Swap in one numeric row
	// for C00001 (seedCommit); the cleanup ETL rerun restores the real rows.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,202607,'SOLO',26) ON CONFLICT DO NOTHING`,
		`DELETE FROM payroll.pyr_att_snapshot_row WHERE pasr_snapshot_id IN (SELECT pas_snapshot_id FROM payroll.pyr_att_snapshot ` + snapWhere + `)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT pas_snapshot_id, 'REKAP', 'C00001', '{"MPAR_DAY_WORK":26}'::jsonb FROM payroll.pyr_att_snapshot ` + snapWhere,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}

	raw, err := sql.Open("pgx", appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	stub := func(in engine.Input) (engine.Result, error) {
		a := dec("1000")
		return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
			Header: &engine.Header{TotTunjangan: a}}, nil
	}
	runs := run.NewServiceWith(postgres.NewDBFromSQL(raw), ruleFunc, audit.NewWriter(), run.Runner{Calc: stub})

	// Commit takes the run advisory lock and parks inside the engine stub; the ETL must then wait on that
	// lock (pg_locks), and once Commit is done it must see the ACTIVE run and leave the snapshot alone.
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	gated := func(in engine.Input) (engine.Result, error) {
		once.Do(func() { close(started); <-release })
		return stub(in)
	}
	runs = run.NewServiceWith(postgres.NewDBFromSQL(raw), ruleFunc, audit.NewWriter(), run.Runner{Calc: gated})
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
	go func() {
		_, e := etl.RunAttLegacy(ctx, etlDB, src, o)
		etlDone <- e
	}()
	waitWaiters(t, "1", etlDone) // ETL queued behind the run advisory lock
	close(release)
	if err := <-commitDone; err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-etlDone; err != nil {
		t.Fatalf("etl: %v", err)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=202607 AND pr_division='SOLO' AND pr_status='ACTIVE' AND pr_kind='ENGINE'`); got != "1" {
		t.Fatalf("run ACTIVE = %s, want 1", got)
	}
	if got := scalar(t, `SELECT pas_checksum||'/'||pas_locked::text FROM payroll.pyr_att_snapshot `+snapWhere); got != "keep/false" {
		t.Fatalf("snapshot berubah setelah Commit mengikat run ACTIVE: %s", got)
	}
	if got := scalar(t, `SELECT count(*)||'/'||min(pasr_nik) FROM payroll.pyr_att_snapshot_row r JOIN payroll.pyr_att_snapshot s ON s.pas_snapshot_id = r.pasr_snapshot_id `+
		strings.Replace(snapWhere, "pas_", "s.pas_", -1)); got != "1/C00001" {
		t.Fatalf("baris snapshot berubah oleh ETL: %s", got)
	}
}
