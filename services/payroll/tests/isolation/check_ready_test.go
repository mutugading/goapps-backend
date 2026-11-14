package isolation

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

const checkReadyPath = "/tmp/check_ready.sql"

// lastMigrationVersion is the highest NNNNNN prefix among migrations/postgres/*.up.sql (what the image ships).
func lastMigrationVersion(t *testing.T) int {
	t.Helper()
	files, err := filepath.Glob("../../migrations/postgres/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob migrations: %v (%d files)", err, len(files))
	}
	sort.Strings(files)
	v, err := strconv.Atoi(strings.SplitN(filepath.Base(files[len(files)-1]), "_", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// runCheckReady runs deploy/check_ready.sql with psql as payroll_ro and returns its output lines.
func runCheckReady(t *testing.T, expectVersion int) []string {
	t.Helper()
	ctx := context.Background()
	if err := pgCtr.CopyFileToContainer(ctx, "../../deploy/check_ready.sql", checkReadyPath, 0o644); err != nil {
		t.Fatalf("copy check_ready.sql: %v", err)
	}
	cmd := fmt.Sprintf("PGPASSWORD=payroll_ro123 psql -h 127.0.0.1 -U payroll_ro -d payroll_db -v ON_ERROR_STOP=1 -v expect_version=%d -f %s",
		expectVersion, checkReadyPath)
	code, out, err := pgCtr.Exec(ctx, []string{"sh", "-c", cmd}, tcexec.Multiplexed())
	if err != nil {
		t.Fatalf("exec psql: %v", err)
	}
	b, _ := io.ReadAll(out) //nolint:errcheck // best-effort; the exit code is checked below
	if code != 0 {
		t.Fatalf("psql exit %d: %s", code, b)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func failLines(lines []string) []string {
	var f []string
	for _, l := range lines {
		if strings.HasPrefix(l, "FAIL ") {
			f = append(f, l)
		}
	}
	return f
}

// withVersionTable creates the golang-migrate table (absent in testcontainers, migrations run as plain SQL).
func withVersionTable(t *testing.T, version int) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := ownDB.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`CREATE TABLE payroll.schema_migrations_payroll (version bigint PRIMARY KEY, dirty boolean NOT NULL)`)
	exec(`INSERT INTO payroll.schema_migrations_payroll VALUES ($1, false)`, version)
	t.Cleanup(func() {
		if _, err := ownDB.ExecContext(ctx, `DROP TABLE IF EXISTS payroll.schema_migrations_payroll`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if err := applyGrants(ctx, ownDB); err != nil { // the version table is swept up by grants.sql like in production
		t.Fatal(err)
	}
}

func hasLine(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// TestCheckReady_Clean: bootstrap + migrations + grants.sql => zero FAIL, and every check reports.
func TestCheckReady_Clean(t *testing.T) {
	v := lastMigrationVersion(t)
	withVersionTable(t, v)
	lines := runCheckReady(t, v)
	if f := failLines(lines); len(f) != 0 {
		t.Fatalf("clean harness: want 0 FAIL, got %v", f)
	}
	for _, want := range []string{"OK version ", "OK dirty ", "OK role payroll_owner", "OK acl_tabel payroll_app", "OK acl_tabel payroll_etl",
		"OK acl_tabel payroll_ro", "OK acl_kolom", "OK acl_sequence", "OK acl_fungsi", "OK rls_tabel", "OK rls_policy", "OK secdef_search_path"} {
		if !hasLine(lines, want) {
			t.Errorf("missing line %q in %v", want, lines)
		}
	}
	// I25: no names/amounts, only DB object names and counts.
	if re := regexp.MustCompile(`(?i)solo one|jkt one|other one`); re.MatchString(strings.Join(lines, "\n")) {
		t.Errorf("I25: output leaks fixture data: %v", lines)
	}
}

// TestCheckReady_Negative: each broken condition yields exactly the FAIL for that check.
func TestCheckReady_Negative(t *testing.T) {
	ctx := context.Background()
	v := lastMigrationVersion(t)
	withVersionTable(t, v)
	mustExec := func(db *sql.DB, q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	restore := func() {
		mustExec(ownDB, `UPDATE payroll.schema_migrations_payroll SET dirty = false`)
		if err := applyGrants(ctx, ownDB); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name   string
		break_ func()
		expect int
		want   string // prefix of the single FAIL line
	}{
		{"revoke ro select", func() { mustExec(ownDB, `REVOKE SELECT ON payroll.pyr_employee FROM payroll_ro`) }, v, "FAIL acl_tabel payroll_ro "},
		{"extra app grant", func() { mustExec(ownDB, `GRANT UPDATE ON payroll.pyr_audit_log TO payroll_app`) }, v, "FAIL acl_tabel payroll_app "},
		{"etl reads salary", func() { mustExec(ownDB, `GRANT SELECT ON payroll.pyr_salary_head TO payroll_etl`) }, v, "FAIL acl_tabel payroll_etl "},
		{"function public", func() { mustExec(ownDB, `GRANT EXECUTE ON FUNCTION payroll.fn_guard_closed() TO PUBLIC`) }, v, "FAIL acl_fungsi "},
		{"dirty", func() { mustExec(ownDB, `UPDATE payroll.schema_migrations_payroll SET dirty = true`) }, v, "FAIL dirty "},
		{"version mismatch", func() {}, v + 1, "FAIL version "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Cleanup(restore)
			c.break_()
			f := failLines(runCheckReady(t, c.expect))
			if len(f) != 1 || !strings.HasPrefix(f[0], c.want) {
				t.Fatalf("want exactly one %q, got %v", c.want, f)
			}
		})
	}
	// Restored state is clean again.
	if f := failLines(runCheckReady(t, v)); len(f) != 0 {
		t.Fatalf("after restore: %v", f)
	}
}

// TestCheckReady_NoRLS: a payroll table without RLS and a SECURITY DEFINER function without search_path.
func TestCheckReady_NoRLSAndSecdef(t *testing.T) {
	ctx := context.Background()
	v := lastMigrationVersion(t)
	withVersionTable(t, v)
	mustExec := func(q string) {
		t.Helper()
		if _, err := ownDB.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE payroll.pyr_zz_norls (x int)`)
	mustExec(`CREATE FUNCTION payroll.fn_zz_secdef() RETURNS int LANGUAGE sql SECURITY DEFINER AS 'SELECT 1'`)
	t.Cleanup(func() {
		mustExec(`DROP FUNCTION IF EXISTS payroll.fn_zz_secdef()`)
		mustExec(`DROP TABLE IF EXISTS payroll.pyr_zz_norls`)
	})
	f := failLines(runCheckReady(t, v))
	var gotRLS, gotSec bool
	for _, l := range f {
		gotRLS = gotRLS || strings.HasPrefix(l, "FAIL rls_tabel ") && strings.Contains(l, "pyr_zz_norls")
		gotSec = gotSec || strings.HasPrefix(l, "FAIL secdef_search_path ") && strings.Contains(l, "fn_zz_secdef")
	}
	if !gotRLS || !gotSec {
		t.Fatalf("want rls_tabel and secdef_search_path FAIL, got %v", f)
	}
}
