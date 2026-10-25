package isolation

// Connection and schema isolation (09§6.1–6.3, I1–I16). Every case runs as the
// role named in the spec and expects a denial. I3/I4 (pg_hba per source host)
// cannot be proven in testcontainers: see TestI3_I4_Staging.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	otherDB      = "goapps"
	otherAppRole = "goapps_app"
	otherAppPW   = "goapps_app123"
	newRole      = "x_nogrant"
	newRolePW    = "x123"
)

var otherDBOnce sync.Once

func openAs(t *testing.T, user, pw, db string) *sql.DB {
	t.Helper()
	d, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, user, pw, db))
	if err != nil {
		t.Fatalf("open %s@%s: %v", user, db, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// superExec runs statements as the cluster superuser on the given database.
func superExec(t *testing.T, db string, stmts ...string) {
	t.Helper()
	s := openAs(t, "postgres", "postgres", db)
	for _, q := range stmts {
		if _, err := s.Exec(q); err != nil {
			t.Fatalf("super exec %q: %v", q, err)
		}
	}
}

// prepareOtherDB creates the "other application" DB and role once, with CONNECT
// revoked from PUBLIC as the spec asks of every non-payroll DB (04§1).
func prepareOtherDB(t *testing.T) {
	t.Helper()
	otherDBOnce.Do(func() { createOtherDB(t) })
}

func createOtherDB(t *testing.T) {
	t.Helper()
	superExec(t, "postgres",
		fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, otherAppRole, otherAppPW),
		fmt.Sprintf(`CREATE DATABASE %s OWNER %s`, otherDB, otherAppRole),
		fmt.Sprintf(`REVOKE CONNECT ON DATABASE %s FROM PUBLIC`, otherDB),
		fmt.Sprintf(`GRANT CONNECT ON DATABASE %s TO %s`, otherDB, otherAppRole),
		fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, newRole, newRolePW),
	)
}

// expectDenied asserts err is SQLSTATE 42501 (insufficient_privilege). Typos in
// object names (42P01/42703) must not pass as "denied".
func expectDenied(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected denial, got success", what)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "42501" {
		return
	}
	t.Fatalf("%s: expected SQLSTATE 42501, got: %v", what, err)
}

// expectWriteDenied accepts 42501 or 25006: payroll_ro is read-only at session level
// (default_transaction_read_only), which fires before the privilege check.
func expectWriteDenied(t *testing.T, what string, err error) {
	t.Helper()
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "42501" || pe.Code == "25006") {
		return
	}
	t.Fatalf("%s: expected 42501 or 25006, got: %v", what, err)
}

// expectConnDenied asserts a connection attempt was refused for privilege or
// pg_hba reasons (the server reports 42501 for a revoked CONNECT).
func expectConnDenied(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected connection denied, got success", what)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "42501" {
		return
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "permission denied for database") || strings.Contains(msg, "pg_hba") {
		return
	}
	t.Fatalf("%s: connection failed for an unexpected reason: %v", what, err)
}

func ping(db *sql.DB) error {
	var one int
	return db.QueryRow(`SELECT 1`).Scan(&one)
}

func TestI1_OtherAppRoleCannotConnectPayroll(t *testing.T) {
	prepareOtherDB(t)
	expectConnDenied(t, "I1", ping(openAs(t, otherAppRole, otherAppPW, "payroll")))
}

func TestI2_PayrollAppCannotConnectOtherDB(t *testing.T) {
	prepareOtherDB(t)
	for _, role := range [][2]string{{"payroll_app", "payroll_app123"}, {"payroll_etl", "payroll_etl123"},
		{"payroll_ro", "payroll_ro123"}, {"payroll_migrator", "payroll_mig123"}} {
		for _, db := range []string{otherDB, "postgres", "template1"} {
			expectConnDenied(t, "I2 "+role[0]+"@"+db, ping(openAs(t, role[0], role[1], db)))
		}
	}
}

// pg_hba restrictions by source host need a real network topology.
func TestI3_I4_Staging(t *testing.T) {
	t.Skip("staging: I3 (payroll_app only from payroll-api host) and I4 (payroll_etl only from payroll-sync host) " +
		"are pg_hba rules (04§13); testcontainers has a single client host, so they are verified on staging")
}

func TestI5_RoleWithoutGrantCannotConnect(t *testing.T) {
	prepareOtherDB(t)
	expectConnDenied(t, "I5", ping(openAs(t, newRole, newRolePW, "payroll")))
}

func TestI6_NonSuperuserCannotCreateCrossDBExtension(t *testing.T) {
	prepareOtherDB(t)
	d := openAs(t, otherAppRole, otherAppPW, otherDB)
	for _, ext := range []string{"dblink", "postgres_fdw"} {
		_, err := d.Exec(`CREATE EXTENSION ` + ext)
		expectDenied(t, "I6 "+ext, err)
	}
	// Even a superuser is stopped inside the payroll DB by the event trigger (04§2).
	s := openAs(t, "postgres", "postgres", "payroll")
	for _, ext := range []string{"dblink", "postgres_fdw"} {
		var avail int
		if err := s.QueryRow(`SELECT count(*) FROM pg_available_extensions WHERE name = $1`, ext).Scan(&avail); err != nil {
			t.Fatal(err)
		}
		if avail == 0 {
			t.Logf("I6: %s not shipped in this image; event trigger path not exercised for it", ext)
			continue
		}
		_, err := s.Exec(`CREATE EXTENSION ` + ext + ` WITH SCHEMA payroll`)
		if err == nil || !strings.Contains(err.Error(), "dilarang") {
			t.Fatalf("I6 superuser %s: expected event trigger denial, got: %v", ext, err)
		}
	}
}

func TestI7_NoCrossDBExtensionInPayroll(t *testing.T) {
	for _, d := range []*sql.DB{ownDB, etlDB} {
		var n int
		if err := d.QueryRow(`SELECT count(*) FROM pg_extension WHERE extname IN ('dblink','postgres_fdw')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("I7: %d cross-db extension(s) present", n)
		}
	}
}

func TestI8_NoForeignServerOrUserMapping(t *testing.T) {
	for _, q := range []string{`SELECT count(*) FROM pg_foreign_server`, `SELECT count(*) FROM pg_user_mappings`} {
		var n int
		if err := ownDB.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if n != 0 {
			t.Fatalf("I8: %q returned %d", q, n)
		}
	}
}

func payrollRoles(t *testing.T) map[string]*sql.DB {
	t.Helper()
	return map[string]*sql.DB{
		"payroll_app":      appDB.DB,
		"payroll_etl":      etlDB,
		"payroll_ro":       openAs(t, "payroll_ro", "payroll_ro123", "payroll"),
		"payroll_migrator": ownDB,
	}
}

func TestI9_NoServerSideFileOrProgramAccess(t *testing.T) {
	ctx := context.Background()
	for name, d := range payrollRoles(t) {
		var n int
		if err := d.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles r WHERE r.rolname = session_user AND
		   (r.rolsuper OR pg_has_role(r.rolname, 'pg_execute_server_program', 'USAGE')
		    OR pg_has_role(r.rolname, 'pg_read_server_files', 'USAGE')
		    OR pg_has_role(r.rolname, 'pg_write_server_files', 'USAGE'))`).Scan(&n); err != nil {
			t.Fatalf("I9 %s: %v", name, err)
		}
		if n != 0 {
			t.Fatalf("I9 %s: holds superuser or server file/program role", name)
		}
		_, err := d.ExecContext(ctx, `COPY (SELECT 1) TO PROGRAM 'true'`)
		expectDenied(t, "I9 COPY TO PROGRAM "+name, err)
		_, err = d.ExecContext(ctx, `COPY (SELECT 1) TO '/tmp/payroll_i9.txt'`)
		expectDenied(t, "I9 COPY TO file "+name, err)
	}
}

func TestI10_EtlCannotReadSalaryDetail(t *testing.T) {
	// Table-level check, independent of schema USAGE: a later grant of USAGE (T020b)
	// must not silently open the table.
	var has bool
	if err := ownDB.QueryRow(`SELECT has_table_privilege('payroll_etl', 'payroll.pyr_salary_detail', 'SELECT')`).Scan(&has); err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("I10: payroll_etl holds SELECT on payroll.pyr_salary_detail")
	}
	var n int
	err := etlDB.QueryRow(`SELECT count(*) FROM payroll.pyr_salary_detail`).Scan(&n)
	expectDenied(t, "I10", err)
}

// I11 expects payroll_etl to read pyr_period / pyr_att_snapshot (lock check), but no
// migration grants it yet (000025/000026 only open master tables). Granting it changes
// isolation permissions, so it is tracked as T020b instead of being decided here.
func TestI11_EtlMayCheckPeriodLockOnly(t *testing.T) {
	t.Skip("gap: payroll_etl has no SELECT on pyr_period/pyr_att_snapshot (09§6.3 I11); follow-up T020b")
}

func TestI12_ReadOnlyRoleCannotWrite(t *testing.T) {
	ro := openAs(t, "payroll_ro", "payroll_ro123", "payroll")
	for _, q := range []string{
		`INSERT INTO payroll.pyr_currency VALUES ('USD', 2, 2)`,
		`UPDATE payroll.pyr_company SET pco_name = 'x'`,
		`DELETE FROM payroll.pyr_division_ref`,
		`INSERT INTO payroll_stg.pyr_stg_sync_batch (psb_source, psb_company_id, psb_division) VALUES ('T',1,'SOLO')`,
		`TRUNCATE payroll.pyr_currency`,
	} {
		_, err := ro.Exec(q)
		expectWriteDenied(t, "I12 "+q, err)
	}
	// "di mana pun": no write privilege on any table, independent of the session flag.
	rows, err := ownDB.Query(`SELECT n.nspname || '.' || c.relname FROM pg_class c
	   JOIN pg_namespace n ON n.oid = c.relnamespace
	  WHERE n.nspname IN ('payroll','payroll_stg') AND c.relkind IN ('r','p')
	    AND (has_table_privilege('payroll_ro', c.oid, 'INSERT') OR has_table_privilege('payroll_ro', c.oid, 'UPDATE')
	      OR has_table_privilege('payroll_ro', c.oid, 'DELETE') OR has_table_privilege('payroll_ro', c.oid, 'TRUNCATE'))`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		_ = rows.Scan(&r)
		t.Errorf("I12: payroll_ro holds a write privilege on %s", r)
	}
}

func TestI13_AppCannotCreateObjects(t *testing.T) {
	for _, q := range []string{
		`CREATE TABLE payroll.x (id int)`,
		`CREATE SCHEMA x_schema`,
		`CREATE TABLE payroll_stg.x (id int)`,
	} {
		_, err := appDB.DB.Exec(q)
		expectDenied(t, "I13 "+q, err)
	}
}

func TestI14_NoRoleCanCreateInPublic(t *testing.T) {
	for name, d := range payrollRoles(t) {
		_, err := d.Exec(`CREATE TABLE public.x (id int)`)
		var pe *pgconn.PgError
		// public is dropped (3F000); if it ever returns, CREATE must still be refused (42501).
		if !errors.As(err, &pe) || (pe.Code != "3F000" && pe.Code != "42501" && pe.Code != "25006") {
			t.Fatalf("I14 %s: expected 3F000, 42501 or 25006, got: %v", name, err)
		}
	}
	var n int
	if err := ownDB.QueryRow(`SELECT count(*) FROM pg_namespace WHERE nspname = 'public'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("I14: schema public still exists in payroll DB")
	}
}

func TestI15_PublicHasNoTablePrivilege(t *testing.T) {
	rows, err := ownDB.Query(`SELECT n.nspname || '.' || c.relname FROM pg_class c
	   JOIN pg_namespace n ON n.oid = c.relnamespace
	  WHERE n.nspname IN ('payroll','payroll_stg') AND c.relkind IN ('r','p','v','m')
	    AND (has_table_privilege('public', c.oid, 'SELECT') OR has_table_privilege('public', c.oid, 'INSERT')
	      OR has_table_privilege('public', c.oid, 'UPDATE') OR has_table_privilege('public', c.oid, 'DELETE'))`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		_ = rows.Scan(&r)
		t.Errorf("I15: PUBLIC has privilege on %s", r)
	}
	var total int
	_ = ownDB.QueryRow(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
	  WHERE n.nspname IN ('payroll','payroll_stg') AND c.relkind IN ('r','p')`).Scan(&total)
	if total == 0 {
		t.Fatal("I15: no tables inspected")
	}
}

func TestI16_SecurityDefinerFunctionsPinSearchPath(t *testing.T) {
	rows, err := ownDB.Query(`SELECT p.oid::regprocedure::text, coalesce(array_to_string(p.proconfig, ','), ''),
	    has_function_privilege('public', p.oid, 'EXECUTE')
	  FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
	 WHERE n.nspname IN ('payroll','payroll_stg') AND p.prosecdef`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var sig, cfg string
		var pub bool
		if err := rows.Scan(&sig, &cfg, &pub); err != nil {
			t.Fatal(err)
		}
		count++
		if !strings.Contains(cfg, "search_path=") {
			t.Errorf("I16: SECURITY DEFINER %s without explicit search_path (%q)", sig, cfg)
		}
		if pub {
			t.Errorf("I16: SECURITY DEFINER %s executable by PUBLIC", sig)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("I16: no SECURITY DEFINER functions found; check is vacuous")
	}
}
