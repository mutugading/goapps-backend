package isolation

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
)

func writeAudit(t *testing.T, ctx context.Context, e domain.Entry) error {
	t.Helper()
	return appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
		return audit.NewWriter().Write(ctx, tx, e)
	})
}

func TestAudit_WriteInScopeMasksPII(t *testing.T) {
	ctx := ctxFor(t, userSolo)
	err := writeAudit(t, ctx, domain.Entry{
		CompanyID: 1, Division: "SOLO", Action: "EMPLOYEE_UPDATE", Entity: "pyr_employee", EntityKey: "S1",
		Before: map[string]any{"npwp": "12.345", "nama": "A"}, After: map[string]any{"npwp": "99.999", "nama": "B"},
		IP: "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	var actor, before string
	if err := ownDB.QueryRowContext(context.Background(),
		`SELECT pal_actor, pal_before::text FROM payroll.pyr_audit_log WHERE pal_entity_key='S1' ORDER BY pal_id DESC LIMIT 1`).
		Scan(&actor, &before); err != nil {
		t.Fatal(err)
	}
	if actor != userSolo {
		t.Fatalf("actor = %q, want %q", actor, userSolo)
	}
	if want := `{"nama": "A", "npwp": "***"}`; before != want {
		t.Fatalf("before = %s, want %s", before, want)
	}
}

func TestAudit_OutsideScopeRejected(t *testing.T) {
	err := writeAudit(t, ctxFor(t, userSolo), domain.Entry{CompanyID: 1, Division: "JAKARTA", Action: "X"})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("err = %v, want RLS violation 42501", err)
	}
}

// DoD: payroll_app cannot UPDATE/DELETE the audit log (append-only).
func TestAudit_AppendOnly(t *testing.T) {
	ctx := ctxFor(t, userSolo)
	if err := writeAudit(t, ctx, domain.Entry{CompanyID: 1, Division: "SOLO", Action: "SEED"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE payroll.pyr_audit_log SET pal_action = 'TAMPER'`,
		`DELETE FROM payroll.pyr_audit_log`,
	} {
		err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, q)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s: err = %v, want permission denied 42501", q, err)
		}
	}
}
