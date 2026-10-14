// Package isolation holds DB isolation tests (09§6.4, I17–I20) against a real
// PostgreSQL 18 started with testcontainers. The schema is built by running
// deploy/payroll/bootstrap.sql then every migrations/postgres/*.up.sql, and the
// assertions connect as payroll_app, exactly like the service.
//
// Gated by INTEGRATION_TEST=true (needs Docker).
package isolation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // driver
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const (
	userSolo  = "user-solo"
	userTwo   = "user-two"
	userAdmin = "user-admin"
	userNone  = "user-none"
)

var (
	appDB *postgres.DB
	ownDB *sql.DB // payroll_migrator (role payroll_owner): fixtures only
)

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("skip isolation tests: INTEGRATION_TEST != true")
		os.Exit(0)
	}
	cleanup, err := setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// setup starts PG18, bootstraps roles/DB, applies migrations and seeds fixtures.
// It returns the container cleanup func.
func setup() (func(), error) {
	ctx := context.Background()
	root, err := filepath.Abs("../../../../..") // workspace root (goapps/)
	if err != nil {
		return nil, err
	}
	bootstrap := filepath.Join(root, "deploy", "payroll", "bootstrap.sql")

	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath: bootstrap, ContainerFilePath: "/bootstrap/bootstrap.sql", FileMode: 0o644,
		}),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	cleanup := func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			fmt.Fprintln(os.Stderr, "terminate:", err)
		}
	}

	if err := runBootstrap(ctx, ctr); err != nil {
		cleanup()
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		cleanup()
		return nil, err
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		cleanup()
		return nil, err
	}
	dsn := func(user, pw string) string {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=payroll sslmode=disable", host, port.Port(), user, pw)
	}

	ownDB, err = sql.Open("pgx", dsn("payroll_migrator", "payroll_mig123"))
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := applyMigrations(ctx, ownDB); err != nil {
		cleanup()
		return nil, err
	}
	if err := seed(ctx, ownDB); err != nil {
		cleanup()
		return nil, err
	}
	rawApp, err := sql.Open("pgx", dsn("payroll_app", "payroll_app123"))
	if err != nil {
		cleanup()
		return nil, err
	}
	appDB = postgres.NewDBFromSQL(rawApp)

	return cleanup, nil
}

func runBootstrap(ctx context.Context, ctr *tcpostgres.PostgresContainer) error {
	code, out, err := ctr.Exec(ctx, []string{"psql", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1",
		"-v", "pw_app=payroll_app123", "-v", "pw_etl=payroll_etl123", "-v", "pw_ro=payroll_ro123",
		"-v", "pw_mig=payroll_mig123", "-f", "/bootstrap/bootstrap.sql"})
	if err != nil {
		return fmt.Errorf("bootstrap exec: %w", err)
	}
	if code != 0 {
		buf := new(strings.Builder)
		if out != nil {
			b := make([]byte, 4096)
			n, _ := out.Read(b) //nolint:errcheck // best-effort diagnostics
			buf.Write(b[:n])
		}
		return fmt.Errorf("bootstrap exit %d: %s", code, buf.String())
	}
	return nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	files, err := filepath.Glob("../../migrations/postgres/*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f) //nolint:gosec // test reads repo migrations
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

func seed(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`INSERT INTO payroll.pyr_currency VALUES ('IDR', 2, -2)`,
		`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('MGT', 'Mutu Gading'), ('OTH', 'Other Co')`,
		`INSERT INTO payroll.pyr_division_ref VALUES (1,'SOLO','Solo'), (1,'JAKARTA','Jakarta'), (2,'SOLO','Solo Other')`,
		`INSERT INTO payroll.pyr_employee (pe_nik, pe_division, pe_nama, pe_orion_sub_acnt, pe_company_id) VALUES
		   ('S1','SOLO','Solo One','x',1), ('J1','JAKARTA','Jkt One','x',1), ('O1','SOLO','Other One','x',2)`,
		`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from) VALUES (1, 'v1', 202601)`,
		`INSERT INTO payroll.pyr_period (pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES
		   (202609,'SOLO','2026-09-01','2026-09-30'), (202609,'JAKARTA','2026-09-01','2026-09-30')`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_yymm, pas_division, pas_source_batch_ids, pas_checksum) VALUES
		   (202609,'SOLO','{1}','c'), (202609,'JAKARTA','{1}','c')`,
		`INSERT INTO payroll.pyr_run (pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status) VALUES
		   (202609,'SOLO','2026-09-30',1,1,'ACTIVE'), (202609,'JAKARTA','2026-09-30',1,2,'ACTIVE')`,
		`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding) VALUES
		   (1,'S1',202609,'SOLO',0,0,0), (2,'J1',202609,'JAKARTA',0,0,0)`,
		`UPDATE payroll.pyr_period p SET pp_active_run_id = r.pr_id FROM payroll.pyr_run r
		   WHERE r.pr_yymm = p.pp_yymm AND r.pr_division = p.pp_division`,
		`INSERT INTO payroll.pyr_rule_constant (prc_rule_set_id, prc_key, prc_value, prc_division) VALUES (1,'GLOBAL',1,NULL)`,
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_company_id, pus_division) VALUES
		   ('` + userSolo + `',1,'SOLO'), ('` + userTwo + `',1,'SOLO'), ('` + userTwo + `',1,'JAKARTA')`,
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_all_scopes) VALUES ('` + userAdmin + `', true)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("seed %q: %w", s[:min(60, len(s))], err)
		}
	}
	return nil
}

// ctxFor resolves scopes like the auth interceptor does and returns a context
// carrying the identity.
func ctxFor(t *testing.T, userID string) context.Context {
	t.Helper()
	ctx := context.Background()
	scopes, err := postgres.NewScopeResolver(appDB).Resolve(ctx, userID)
	if err != nil {
		t.Fatalf("resolve %s: %v", userID, err)
	}
	return scope.WithIdentity(ctx, scope.Identity{UserID: userID, Scopes: scopes})
}

func queryStrings(t *testing.T, ctx context.Context, q string) []string {
	t.Helper()
	var out []string
	err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	sort.Strings(out)
	return out
}

func TestResolver_Scopes(t *testing.T) {
	tests := []struct {
		user string
		want string
	}{
		{userSolo, "1:SOLO"},
		{userTwo, "1:JAKARTA,1:SOLO"},
		{userAdmin, "1:JAKARTA,1:SOLO,2:SOLO"},
		{userNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.user, func(t *testing.T) {
			id, _ := scope.FromContext(ctxFor(t, tt.user))
			if got := scope.Join(id.Scopes); got != tt.want {
				t.Fatalf("scopes = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRLS_EmployeeVisibility(t *testing.T) {
	const q = `SELECT pe_company_id || ':' || pe_nik FROM payroll.pyr_employee`
	tests := []struct {
		name string
		user string
		want string
	}{
		{"SOLO user does not read JAKARTA", userSolo, "1:S1"},
		{"two scopes read both", userTwo, "1:J1,1:S1"},
		{"admin reads all", userAdmin, "1:J1,1:S1,2:O1"},
		{"no scope reads nothing", userNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(queryStrings(t, ctxFor(t, tt.user), q), ",")
			if got != tt.want {
				t.Fatalf("rows = %q, want %q", got, tt.want)
			}
		})
	}
}

// I17: app.scopes = '1:SOLO'; SELECT header JAKARTA -> 0 rows.
func TestI17_SoloCannotReadJakartaHeader(t *testing.T) {
	got := queryStrings(t, ctxFor(t, userSolo),
		`SELECT psh_nik FROM payroll.pyr_salary_head WHERE psh_division = 'JAKARTA'`)
	if len(got) != 0 {
		t.Fatalf("I17: got %v, want 0 rows", got)
	}
	// Compatibility view must not bypass RLS either (security_invoker).
	got = queryStrings(t, ctxFor(t, userSolo), `SELECT pm_nik FROM payroll.v_pyr_mgt_payroll_main`)
	if strings.Join(got, ",") != "S1" {
		t.Fatalf("I17 view: got %v, want [S1]", got)
	}
}

// I18: no app.scopes set -> 0 rows (fail closed), on a raw payroll_app session.
func TestI18_NoScopeSetFailsClosed(t *testing.T) {
	for _, tbl := range []string{"pyr_employee", "pyr_salary_head", "pyr_run", "pyr_period", "pyr_rule_constant"} {
		var n int
		if err := appDB.QueryRowContext(context.Background(), "SELECT count(*) FROM payroll."+tbl).Scan(&n); err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
		if n != 0 {
			t.Fatalf("I18 %s: %d rows without app.scopes, want 0", tbl, n)
		}
	}
}

// I19: INSERT a JAKARTA row with SOLO scope -> rejected by WITH CHECK.
func TestI19_InsertOutsideScopeRejected(t *testing.T) {
	ctx := ctxFor(t, userSolo)
	err := appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_addition (pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount)
			VALUES (202609, 'J1', 'JAKARTA', 'Tunjangan', 1, 1000)`)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("I19: err = %v, want RLS violation 42501", err)
	}

	// Same insert inside scope succeeds (rolled back so other tests are unaffected).
	errRollback := errors.New("rollback")
	err = appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_addition (pa_yymm, pa_nik, pa_division, pa_type, pa_id, pa_amount)
			VALUES (202609, 'S1', 'SOLO', 'Tunjangan', 1, 1000)`); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("I19 in-scope insert: %v", err)
	}
}

// I20: no payroll table with a *_division column lacks RLS.
func TestI20_AllDivisionTablesHaveRLS(t *testing.T) {
	rows, err := ownDB.QueryContext(context.Background(), `
		SELECT c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname LIKE '%\_division'
		WHERE n.nspname = 'payroll' AND c.relkind = 'r' AND NOT c.relrowsecurity`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		missing = append(missing, s)
	}
	if len(missing) != 0 {
		t.Fatalf("I20: tables without RLS: %v", missing)
	}
}

// Global rows (division NULL) are visible to any user with a scope in that
// company, never to a user without scope.
func TestRLS_GlobalDivisionNull(t *testing.T) {
	const q = `SELECT prc_key FROM payroll.pyr_rule_constant`
	if got := queryStrings(t, ctxFor(t, userSolo), q); strings.Join(got, ",") != "GLOBAL" {
		t.Fatalf("solo: got %v, want [GLOBAL]", got)
	}
	if got := queryStrings(t, ctxFor(t, userNone), q); len(got) != 0 {
		t.Fatalf("none: got %v, want 0 rows", got)
	}
}

// User scope rows are private to their owner (p_self).
func TestRLS_UserScopeSelfOnly(t *testing.T) {
	got := queryStrings(t, ctxFor(t, userSolo), `SELECT DISTINCT pus_user_id FROM payroll.pyr_user_scope`)
	if strings.Join(got, ",") != userSolo {
		t.Fatalf("got %v, want only %s", got, userSolo)
	}
}

// ScopedTx without identity must refuse to run.
func TestScopedTx_NoIdentity(t *testing.T) {
	err := appDB.ScopedTx(context.Background(), func(*sql.Tx) error { return nil })
	if !errors.Is(err, scope.ErrNoIdentity) {
		t.Fatalf("err = %v, want ErrNoIdentity", err)
	}
}

// Settings are transaction-local: a pooled connection must not leak scope.
func TestScopedTx_SettingIsTxLocal(t *testing.T) {
	ctx := ctxFor(t, userAdmin)
	if err := appDB.ScopedTx(ctx, func(*sql.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var v sql.NullString
	if err := appDB.QueryRowContext(context.Background(), `SELECT current_setting('app.scopes', true)`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v.Valid && v.String != "" {
		t.Fatalf("app.scopes leaked outside tx: %q", v.String)
	}
}
