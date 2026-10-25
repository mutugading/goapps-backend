// Package etl tests the master-component ETL (T009a, D-22) against a real PostgreSQL 18
// (testcontainers): idempotency, row counts vs source, level_rule vs 02§5.1.
//
// Gated by INTEGRATION_TEST=true (needs Docker).
package etl

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

var etlDB, ownDB *sql.DB

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("skip etl tests: INTEGRATION_TEST != true")
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

func setup() (func(), error) {
	ctx := context.Background()
	root, err := filepath.Abs("../../../../..") // workspace root (goapps/)
	if err != nil {
		return nil, err
	}
	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("postgres"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath: filepath.Join(root, "deploy", "payroll", "bootstrap.sql"), ContainerFilePath: "/bootstrap/bootstrap.sql", FileMode: 0o644,
		}),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = testcontainers.TerminateContainer(ctr) }

	code, _, err := ctr.Exec(ctx, []string{"psql", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1",
		"-v", "pw_app=payroll_app123", "-v", "pw_etl=payroll_etl123", "-v", "pw_ro=payroll_ro123",
		"-v", "pw_mig=payroll_mig123", "-f", "/bootstrap/bootstrap.sql"})
	if err != nil || code != 0 {
		return cleanup, fmt.Errorf("bootstrap exit %d: %v", code, err)
	}
	host, _ := ctr.Host(ctx)
	port, _ := ctr.MappedPort(ctx, "5432/tcp")
	dsn := func(user, pw string) string {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=payroll sslmode=disable", host, port.Port(), user, pw)
	}
	if ownDB, err = sql.Open("pgx", dsn("payroll_migrator", "payroll_mig123")); err != nil {
		return cleanup, err
	}
	files, _ := filepath.Glob("../../migrations/postgres/*.up.sql")
	sort.Strings(files)
	for _, f := range files {
		b, _ := os.ReadFile(f) //nolint:gosec // test reads repo migrations
		if _, err = ownDB.ExecContext(ctx, string(b)); err != nil {
			return cleanup, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	for _, q := range []string{
		`INSERT INTO payroll.pyr_currency VALUES ('IDR', 2, -2)`,
		`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('MGT','Mutu Gading')`,
		`INSERT INTO payroll.pyr_division_ref VALUES (1,'SOLO','Solo'),(1,'JAKARTA','Jakarta')`,
	} {
		if _, err = ownDB.ExecContext(ctx, q); err != nil {
			return cleanup, err
		}
	}
	etlDB, err = sql.Open("pgx", dsn("payroll_etl", "payroll_etl123"))
	return cleanup, err
}

func scalar(t *testing.T, q string) string {
	t.Helper()
	var s string
	if err := ownDB.QueryRow(q).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestETL_IdempotentAndCounts(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.Options{CompanyID: 1, ValidFrom: 202001, ExpectMaster: 4, ExpectTemplate: 4}

	snap := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(pc_sysid||pc_code||pc_name||pc_tax_class||pc_default_sts, ',' ORDER BY pc_sysid) FROM payroll.pyr_component) ||
		  (SELECT string_agg(pclm_legacy_id||pclm_legacy_division||pclm_sysid, ',' ORDER BY 1) FROM payroll.pyr_component_legacy_map) ||
		  (SELECT string_agg(pct_template||pct_component_sysid||coalesce(pct_amount::text,''), ',' ORDER BY 1) FROM payroll.pyr_component_template) ||
		  (SELECT string_agg(plr_level||plr_otinwork_rate, ',' ORDER BY 1) FROM payroll.pyr_level_rule))`)
	}
	if _, err := etl.Run(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	first := snap()
	reps, err := etl.Run(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	if snap() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for _, r := range reps {
		if !r.OK() {
			t.Errorf("%s src=%d dst=%d", r.Table, r.SrcRows, r.DstRows)
		}
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_component`); n != "4" {
		t.Errorf("pyr_component = %s, want 4", n)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_component_legacy_map`); n != "8" { // 4 komponen × 2 divisi
		t.Errorf("legacy_map = %s, want 8", n)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_component_template`); n != "4" {
		t.Errorf("template = %s, want 4", n)
	}
	if n := scalar(t, `SELECT count(DISTINCT pclm_sysid) FROM payroll.pyr_component_legacy_map`); n != "4" {
		t.Errorf("1 sysid per (type,id): %s", n)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_level_rule`); n != "15" {
		t.Errorf("level_rule = %s, want 15", n)
	}
	if c := scalar(t, `SELECT pc_tax_class||pc_default_sts FROM payroll.pyr_component WHERE pc_code='T034'`); c != "TAXABLED" {
		t.Errorf("T034 = %s", c)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_migration_log WHERE pml_ok`); n != "6" {
		t.Errorf("migration_log = %s, want 6 (3 tabel × 2 run)", n)
	}
}

func TestETL_RollbackOnMissingMap(t *testing.T) {
	dir := t.TempDir()
	master := "MPCM_TYPE|MPCM_ID|MPCM_NAME|MPCM_STS|MPCM_TAX|MPCM_SERIAL|MPCM_FIX_INCOME|MPCM_TAX_PAID_COMPANY|MPCM_TAX_SUBSIDY\nTunjangan|900|Baru|F|2|1||N|N\n"
	tmpl := "MPCT_TEMPLATE|MPCT_TYPE|MPCT_ID|MPCT_NAME|MPCT_STS|MPCT_TAX|MPCT_AMOUNT|MPCT_TAX_PAID_COMPANY\nDriver|Tunjangan|901|X|F|2|1|N\n"
	_ = os.WriteFile(filepath.Join(dir, "MGT_PAYROLL_COMPNT_MASTER.psv"), []byte(master), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MGT_PAYROLL_COMPNT_TEMPLATE.psv"), []byte(tmpl), 0o600)
	if _, err := etl.Run(context.Background(), etlDB, etl.PSVDir{Dir: dir}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err == nil {
		t.Fatal("template tanpa peta sysid harus gagal")
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_component WHERE pc_code='T900'`); n != "0" {
		t.Errorf("rollback gagal, T900 tersisa: %s", n)
	}
}

func TestETL_InputsIdempotentAndCounts(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.InputOptions{CompanyID: 1, ExpectAddition: 3, ExpectAbnormal: 2}
	snap := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(psv_vs_code||psv_code||coalesce(psv_name,''), ',' ORDER BY 1) FROM payroll.pyr_static_value) ||
		  (SELECT string_agg(prc_key||coalesce(prc_division,'*')||prc_value, ',' ORDER BY 1) FROM payroll.pyr_rule_constant) ||
		  (SELECT string_agg(pa_nik||pa_type||pa_id||pa_amount||pa_source, ',' ORDER BY 1) FROM payroll.pyr_addition) ||
		  (SELECT string_agg(pta_nik||pta_amount, ',' ORDER BY 1) FROM payroll.pyr_tax_adj) ||
		  (SELECT string_agg(phd_date::text, ',' ORDER BY 1) FROM payroll.pyr_holiday) ||
		  (SELECT string_agg(pap_date::text, ',' ORDER BY 1) FROM payroll.pyr_abnormal_premi))`)
	}
	if _, err := etl.RunInputs(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	first := snap()
	reps, err := etl.RunInputs(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	if snap() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for _, r := range reps {
		if !r.OK() {
			t.Errorf("%s src=%d dst=%d", r.Table, r.SrcRows, r.DstRows)
		}
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_static_value`:                                    "3",
		`SELECT count(*) FROM payroll.pyr_rule_constant`:                                   "13",
		`SELECT count(*) FROM payroll.pyr_attd_days`:                                       "2",
		`SELECT count(*) FROM payroll.pyr_subsidi_pph`:                                     "2",
		`SELECT count(*) FROM payroll.pyr_holiday`:                                         "2",
		`SELECT count(*) FROM payroll.pyr_addition WHERE pa_source='LEGACY'`:               "3",
		`SELECT sum(pa_amount)::text FROM payroll.pyr_addition`:                            "1750000.75",
		`SELECT pta_amount::text FROM payroll.pyr_tax_adj`:                                 "1250000.50",
		`SELECT count(*) FROM payroll.pyr_abnormal_premi`:                                  "2",
		`SELECT count(*) FROM payroll.pyr_batal_premi`:                                     "2",
		`SELECT prc_value::text FROM payroll.pyr_rule_constant WHERE prc_key='DAYS_MONTH'`: "30.000000",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
}
