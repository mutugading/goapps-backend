package isolation

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestGrants_RefusesNonOwner: grants.sql run by anyone but payroll_owner must fail, not WARN and exit 0
// (a GRANT by a non-owner is only a warning), so the deploy Job cannot go green without grants (T028n5).
func TestGrants_RefusesNonOwner(t *testing.T) {
	body, err := os.ReadFile("../../deploy/grants.sql") //nolint:gosec // fixed in-repo path
	if err != nil {
		t.Fatal(err)
	}
	s := openAs(t, "postgres", "postgres", "payroll_db")
	_, err = s.ExecContext(context.Background(), string(body))
	if err == nil || !strings.Contains(err.Error(), "must run as payroll_owner") {
		t.Fatalf("grants.sql as superuser: want guard error, got %v", err)
	}
}

// TestGrants_MigrationsTableReadOnlyForApp: the golang-migrate version table sits in schema payroll (staging/
// production) and is swept up by GRANT ON ALL TABLES; grants.sql must leave payroll_app with SELECT only.
func TestGrants_MigrationsTableReadOnlyForApp(t *testing.T) {
	ctx := context.Background()
	if _, err := ownDB.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS payroll.schema_migrations_payroll (version bigint PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.ExecContext(ctx, `DROP TABLE IF EXISTS payroll.schema_migrations_payroll`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if err := applyGrants(ctx, ownDB); err != nil {
		t.Fatal(err)
	}
	for priv, want := range map[string]bool{"SELECT": true, "INSERT": false, "UPDATE": false, "DELETE": false, "TRUNCATE": false} {
		var got bool
		if err := ownDB.QueryRowContext(ctx,
			`SELECT has_table_privilege('payroll_app', 'payroll.schema_migrations_payroll', $1)`, priv).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("payroll_app %s on schema_migrations_payroll = %v, want %v", priv, got, want)
		}
	}
}
