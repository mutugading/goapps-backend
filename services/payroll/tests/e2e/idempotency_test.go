package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/idempotency"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const (
	hashA = "a000000000000000000000000000000000000000000000000000000000000000"
	hashB = "b000000000000000000000000000000000000000000000000000000000000000"
)

// Idempotency-Key store against real PG18 + RLS (D-19, T007e).
func TestIdempotencyStore(t *testing.T) {
	ctx := context.Background()
	st := postgres.NewIdempotencyStore(appDB)
	k := idempotency.Key{UserID: userRW, Method: "/payroll.v1.PayrollService/CreateCompany", Key: "k-e2e"}

	claims := map[idempotency.Key]string{}
	begin := func(k idempotency.Key, req, sc string) (idempotency.State, []byte) {
		t.Helper()
		s, c, b, err := st.Begin(ctx, k, req, sc)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if s == idempotency.Started {
			claims[k] = c
		}
		return s, b
	}

	if s, _ := begin(k, hashA, hashA); s != idempotency.Started {
		t.Fatalf("first Begin = %v, want Started", s)
	}
	if s, _ := begin(k, hashA, hashA); s != idempotency.InProgress {
		t.Fatalf("second Begin before Complete = %v, want InProgress", s)
	}
	if err := st.Complete(ctx, k, claims[k], []byte("resp")); err != nil {
		t.Fatal(err)
	}
	if s, b := begin(k, hashA, hashA); s != idempotency.Replay || string(b) != "resp" {
		t.Fatalf("after Complete = %v %q, want Replay resp", s, b)
	}
	if s, _ := begin(k, hashB, hashA); s != idempotency.Conflict {
		t.Fatalf("other body = %v, want Conflict", s)
	}
	if s, _ := begin(k, hashA, hashB); s != idempotency.Conflict {
		t.Fatalf("other scope = %v, want Conflict", s)
	}

	// Same key string, another user: independent (RLS p_self).
	other := k
	other.UserID = userRO
	if s, _ := begin(other, hashB, hashB); s != idempotency.Started {
		t.Fatalf("other user = %v, want Started", s)
	}

	// Expired rows are taken over by a fresh claim.
	if _, err := ownDB.ExecContext(ctx, `UPDATE payroll.pyr_idempotency_key SET pik_expires_at = now() - interval '1 second'
		WHERE pik_user_id = $1 AND pik_key = $2`, userRW, k.Key); err != nil {
		t.Fatal(err)
	}
	if s, _ := begin(k, hashB, hashB); s != idempotency.Started {
		t.Fatalf("after expiry = %v, want Started", s)
	}

	// Abort frees an IN_PROGRESS claim; a DONE row is never aborted.
	if err := st.Abort(ctx, k, claims[k]); err != nil {
		t.Fatal(err)
	}
	if s, _ := begin(k, hashB, hashB); s != idempotency.Started {
		t.Fatalf("after Abort = %v, want Started", s)
	}
	if err := st.Complete(ctx, k, claims[k], []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := st.Abort(ctx, k, claims[k]); err != nil {
		t.Fatal(err)
	}
	if s, _ := begin(k, hashB, hashB); s != idempotency.Replay {
		t.Fatalf("DONE row must survive Abort, got %v", s)
	}

	// A stale claim token cannot complete or abort a newer claim.
	old := claims[k]
	begin(k, hashB, hashB) // DONE row: replay, claim unchanged
	fresh := idempotency.Key{UserID: userRW, Method: k.Method, Key: "k-token"}
	begin(fresh, hashA, hashA)
	if err := st.Complete(ctx, fresh, old, []byte("late")); err != nil {
		t.Fatal(err)
	}
	if err := st.Abort(ctx, fresh, old); err != nil {
		t.Fatal(err)
	}
	if s, _ := begin(fresh, hashA, hashA); s != idempotency.InProgress {
		t.Fatalf("foreign claim token changed the row: %v, want InProgress", s)
	}

	// Stale IN_PROGRESS (crashed request) is taken over.
	stale := idempotency.Key{UserID: userRW, Method: k.Method, Key: "k-stale"}
	begin(stale, hashA, hashA)
	if _, err := ownDB.ExecContext(ctx, `UPDATE payroll.pyr_idempotency_key SET pik_created_at = now() - interval '3 minutes'
		WHERE pik_key = 'k-stale'`); err != nil {
		t.Fatal(err)
	}
	if s, _ := begin(stale, hashA, hashA); s != idempotency.Started {
		t.Fatalf("stale IN_PROGRESS = %v, want Started", s)
	}

	// RLS: payroll_app without app.user_id sees nothing.
	var n int
	if err := appDB.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_idempotency_key`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no session user must see 0 rows, got %d (%v)", n, err)
	}
}

// T007f: purge removes keys expired beyond the grace window for every user, keeps the rest.
func TestIdempotencyPurge(t *testing.T) {
	ctx := context.Background()
	st := postgres.NewIdempotencyStore(appDB)
	m := "/payroll.v1.PayrollService/CreateCompany"
	for _, k := range []idempotency.Key{
		{UserID: userRW, Method: m, Key: "p-old"}, {UserID: userRO, Method: m, Key: "p-old"},
		{UserID: userRW, Method: m, Key: "p-recent"}, {UserID: userRW, Method: m, Key: "p-live"},
	} {
		if _, _, _, err := st.Begin(ctx, k, hashA, hashA); err != nil {
			t.Fatal(err)
		}
	}
	for key, age := range map[string]string{"p-old": "2 days", "p-recent": "1 hour"} {
		if _, err := ownDB.ExecContext(ctx, `UPDATE payroll.pyr_idempotency_key SET pik_expires_at = now() - $1::interval
			WHERE pik_key = $2`, age, key); err != nil {
			t.Fatal(err)
		}
	}
	n, err := st.Purge(ctx, 24*time.Hour)
	if err != nil || n < 2 {
		t.Fatalf("Purge = %d, %v; want >= 2 (both users' p-old)", n, err)
	}
	var left []string
	rows, err := ownDB.QueryContext(ctx, `SELECT pik_key FROM payroll.pyr_idempotency_key WHERE pik_key LIKE 'p-%' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		left = append(left, k)
	}
	if strings.Join(left, ",") != "p-live,p-recent" {
		t.Fatalf("left = %v, want [p-live p-recent]", left)
	}
}
