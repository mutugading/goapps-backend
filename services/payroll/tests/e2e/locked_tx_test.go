package e2e

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

const lockedNS = "t033d.test"

func advisoryLocks(t *testing.T) string {
	t.Helper()
	return scalar(t, `SELECT count(*)::text FROM pg_locks WHERE locktype='advisory' AND classid::bigint = (hashtext('`+lockedNS+`')::bigint & 4294967295)`)
}

// T033d: ScopedTxOptsLocked takes the lock BEFORE the REPEATABLE READ snapshot: a waiter sees what the previous holder
// committed while it waited. With a transaction-level lock (taken after the snapshot) it would read the old state.
func TestScopedTxOptsLocked_SnapshotAfterLock(t *testing.T) {
	ctx := ctxFor(t, userRW)
	opts := &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
	t.Cleanup(func() { _, _ = ownDB.Exec(`DELETE FROM payroll.pyr_currency WHERE pcy_code='ZZT'`) }) //nolint:errcheck // cleanup
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_currency WHERE pcy_code='ZZT'`); err != nil {
		t.Fatal(err)
	}
	holding, release := make(chan struct{}), make(chan struct{})
	doneA := make(chan error, 1)
	go func() {
		doneA <- appDB.ScopedTxOptsLocked(ctx, opts, lockedNS, "k", func(tx *sql.Tx) error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	seen := make(chan string, 1)
	doneB := make(chan error, 1)
	go func() {
		doneB <- appDB.ScopedTxOptsLocked(ctx, opts, lockedNS, "k", func(tx *sql.Tx) error {
			var n string
			err := tx.QueryRowContext(ctx, `SELECT count(*)::text FROM payroll.pyr_currency WHERE pcy_code='ZZT'`).Scan(&n)
			seen <- n
			return err
		})
	}()
	time.Sleep(300 * time.Millisecond) // B is now waiting for the lock
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_currency (pcy_code, pcy_minor_unit, pcy_takehome_round) VALUES ('ZZT', 2, 0)`); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-doneA; err != nil {
		t.Fatal(err)
	}
	if err := <-doneB; err != nil {
		t.Fatal(err)
	}
	if n := <-seen; n != "1" {
		t.Fatalf("waiter did not see the row committed while it waited (snapshot taken before the lock): %s", n)
	}
	if got := advisoryLocks(t); got != "0" {
		t.Fatalf("advisory locks left = %s", got)
	}
}

// T033d: the lock is released when fn fails or the context is cancelled, and the connection returns to the pool
// (with one open connection a leak would block the next call).
func TestScopedTxOptsLocked_ReleasesLock(t *testing.T) {
	ctx := ctxFor(t, userRW)
	appDB.SetMaxOpenConns(1)
	t.Cleanup(func() { appDB.SetMaxOpenConns(0) })
	boom := errors.New("boom")
	if err := appDB.ScopedTxOptsLocked(ctx, nil, lockedNS, "k", func(*sql.Tx) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("fn error: %v", err)
	}
	if got := advisoryLocks(t); got != "0" {
		t.Fatalf("lock left after fn error = %s", got)
	}
	cctx, cancel := context.WithCancel(ctx)
	err := appDB.ScopedTxOptsLocked(cctx, nil, lockedNS, "k", func(*sql.Tx) error { cancel(); return nil })
	_ = err // commit may fail with context canceled; only the cleanup matters
	ok := make(chan error, 1)
	go func() { ok <- appDB.ScopedTxOptsLocked(ctx, nil, lockedNS, "k", func(*sql.Tx) error { return nil }) }()
	select {
	case err := <-ok:
		if err != nil {
			t.Fatalf("after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock or connection leaked after context cancel")
	}
	if got := advisoryLocks(t); got != "0" {
		t.Fatalf("lock left after cancel = %s", got)
	}
}
