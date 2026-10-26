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
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

var etlDB, ownDB *sql.DB

// appDSN connects as payroll_app (the service role), for loader tests.
var appDSN string

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
	appDSN = dsn("payroll_app", "payroll_app123")
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

func TestETL_AttLegacySnapshots(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	o := etl.AttLegacyOptions{CompanyID: 1, ExpectRekap: 5, ExpectAdd: 1, ExpectS: 3}
	state := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(pas_yymm||pas_division||pas_checksum||pas_locked, ',' ORDER BY 1) FROM payroll.pyr_att_snapshot) ||
		  (SELECT string_agg(pasr_kind||pasr_nik||pasr_data::text, ',' ORDER BY 1) FROM payroll.pyr_att_snapshot_row))`)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, o); err != nil {
		t.Fatal(err)
	}
	first := state()
	reps, err := etl.RunAttLegacy(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for _, r := range reps {
		if !r.OK() {
			t.Errorf("%s src=%d dst=%d exc=%d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
	}
	if len(reps[0].Exceptions) != 1 {
		t.Errorf("exception yymm kotor = %d, want 1", len(reps[0].Exceptions))
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_locked`:                                                                                               "3",
		`SELECT count(*) FROM payroll.pyr_att_snapshot`:                                                                                                                "3",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='REKAP'`:                                                                                    "4",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='ADD'`:                                                                                      "1",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='SCH'`:                                                                                      "2",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_premi_fail IS NOT NULL`:                                                                          "0",
		`SELECT (pasr_data->>'MPAR_LEMBUR_2') FROM payroll.pyr_att_snapshot_row WHERE pasr_nik='2026-0001' AND pasr_kind='REKAP' AND pasr_data->>'MPAR_DAY_WORK'='24'`: "0.5",
		`SELECT (pasr_data->'SCHEDULES'->>'S') FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='SCH' AND pasr_nik='2026-0001'`:                                       "14",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_data ? 'MPAR_CUTI' AND pasr_data->>'MPAR_CUTI' IS NULL`:                                          "3",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
}

func TestETL_SalaryLegacyRuns(t *testing.T) {
	ctx := context.Background()
	src := etl.PSVDir{Dir: "testdata"}
	// komponen & snapshot LEGACY (T009a/T009d) adalah prasyarat; keduanya idempotent.
	if _, err := etl.Run(ctx, etlDB, src, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	if _, err := etl.RunAttLegacy(ctx, etlDB, src, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	o := etl.SalaryLegacyOptions{CompanyID: 1, ExpectMain: 5, ExpectDetail: 7}
	state := func() string {
		return scalar(t, `SELECT md5(
		  (SELECT string_agg(pr_yymm||pr_division||pr_kind||pr_status||pr_rule_set_id||pr_emp_count||pr_line_count||pr_tot_tunjangan, ',' ORDER BY 1) FROM payroll.pyr_run) ||
		  (SELECT string_agg(psh_nik||psh_yymm||psh_net_pay, ',' ORDER BY 1) FROM payroll.pyr_salary_head) ||
		  (SELECT string_agg(psd_nik||psd_id||coalesce(psd_amount::text,'null')||psd_component_sysid, ',' ORDER BY 1) FROM payroll.pyr_salary_detail))`)
	}
	reps, err := etl.RunSalaryLegacy(ctx, etlDB, src, o)
	if err != nil {
		t.Fatal(err)
	}
	first := state()
	reps2, err := etl.RunSalaryLegacy(ctx, etlDB, src, o)
	if err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	for i, r := range reps {
		if !r.OK() || !reps2[i].OK() {
			t.Errorf("%s src=%d dst=%d exc=%d", r.Table, r.SrcRows, r.DstRows, len(r.Exceptions))
		}
	}
	// 202607 SOLO di-skip (D-35.1). MAIN: 202608 tanpa snapshot + 2027 kotor; DETAIL: 202608 (tanpa snapshot) + 2027
	if len(reps[0].Exceptions) != 2 || len(reps[1].Exceptions) != 2 {
		t.Errorf("exception main=%d detail=%d, want 2/2", len(reps[0].Exceptions), len(reps[1].Exceptions))
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_kind='LEGACY' AND pr_status='ACTIVE' AND pr_rule_set_id=0`:       "1",
		`SELECT count(*) FROM payroll.pyr_period WHERE pp_status='CLOSED' AND pp_active_run_id IS NOT NULL`:             "1",
		`SELECT count(*) FROM payroll.pyr_salary_head`:                                                                  "2",
		`SELECT count(*) FROM payroll.pyr_salary_detail`:                                                                "4",
		`SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_formula='LEGACY' AND psd_component_sysid IS NOT NULL`: "4",
		`SELECT count(*) FROM payroll.pyr_salary_detail WHERE psd_amount IS NULL`:                                       "1",
		`SELECT psh_net_pay::text FROM payroll.pyr_salary_head WHERE psh_nik='2026-0001' AND psh_yymm=202606`:           "5400000.00",
		`SELECT pr_tot_tunjangan::text FROM payroll.pyr_run WHERE pr_yymm=202606 AND pr_division='SOLO'`:                "9500000.50",
		`SELECT psh_jam_lembur->>'lembur1' FROM payroll.pyr_salary_head WHERE psh_nik='2026-0001' AND psh_yymm=202606`:  "1.5",
		`SELECT count(*) FROM payroll.v_pyr_mgt_payroll_main WHERE pm_status=3`:                                         "2",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// periode CLOSED tetap menolak mutasi sesudah load (04§9), juga untuk pemilik skema.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount)
		   SELECT pr_id, '2026-0001', 'Tunjangan', 77, 1 FROM payroll.pyr_run WHERE pr_yymm=202606`,
		`UPDATE payroll.pyr_salary_head SET psh_rounding = 1 WHERE psh_yymm=202606`,
		`DELETE FROM payroll.pyr_salary_detail WHERE psd_nik='2026-0001'`,
	} {
		if _, err := ownDB.Exec(q); err == nil {
			t.Errorf("periode CLOSED menerima mutasi: %s", q)
		}
	}
	res, err := etlDB.Exec(`UPDATE payroll.pyr_period SET pp_status='OPEN' WHERE pp_yymm=202606`)
	if err == nil {
		if n, _ := res.RowsAffected(); n != 0 {
			t.Errorf("payroll_etl membuka kembali %d periode CLOSED", n)
		}
	}
	if got := scalar(t, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_yymm=202606 AND pp_division='SOLO'`); got != "CLOSED" {
		t.Errorf("status periode 202606 = %s, want CLOSED", got)
	}
}

func TestETL_SalaryLegacyGuards(t *testing.T) {
	ctx := context.Background()
	// periode milik engine (OPEN, run ENGINE) di 202609/SOLO: ETL tidak boleh menyentuhnya.
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (1,202609,'SOLO','2026-08-26','2026-09-25')`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked) VALUES (1,202609,'SOLO','{1}','x',true)`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
		   SELECT 1,202609,'SOLO','2026-09-25',1,pas_snapshot_id,'RUNNING','ENGINE' FROM payroll.pyr_att_snapshot WHERE pas_yymm=202609`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := etlDB.Exec(`UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_yymm=202609`); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT pp_status::text FROM payroll.pyr_period WHERE pp_yymm=202609`); got != "OPEN" {
		t.Errorf("payroll_etl menutup periode milik engine: %s", got)
	}
	if _, err := etlDB.Exec(`INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_nik, psh_yymm, psh_division, psh_tot_tunjangan, psh_tot_potongan, psh_rounding)
	   SELECT pr_id,'x',202609,'SOLO',0,0,0 FROM payroll.pyr_run WHERE pr_yymm=202609`); err == nil {
		t.Error("payroll_etl menambah head ke run ENGINE")
	}
	if _, err := etlDB.Exec(`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status, pr_kind)
	   SELECT 1,202609,'SOLO','2026-09-25',1,pas_snapshot_id,'DRY','ENGINE' FROM payroll.pyr_att_snapshot WHERE pas_yymm=202609`); err == nil {
		t.Error("payroll_etl membuat run ENGINE")
	}
	var n int
	if err := etlDB.QueryRow(`SELECT count(*) FROM payroll.pyr_salary_head`).Scan(&n); err == nil {
		t.Error("payroll_etl dapat SELECT pyr_salary_head")
	}
	if err := etlDB.QueryRow(`SELECT count(pr_tot_tunjangan) FROM payroll.pyr_run`).Scan(&n); err == nil {
		t.Error("payroll_etl dapat SELECT total gaji di pyr_run")
	}

	// periode tertahan: satu DETAIL tanpa peta sysid menahan seluruh periode (CLOSED tidak bisa menerima baris susulan).
	for _, q := range []string{
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked) VALUES (1,202610,'SOLO','{}','x',true)`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	head := "PM_YYMM|PM_NIK|PM_DIVISION|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING\n202610|2026-0001|SOLO|10|0|0\n"
	det := "PD_YYMM|PD_NIK|PD_TYPE|PD_NO|PD_DIVISION|PD_AMOUNT\n202610|2026-0001|Tunjangan|1|SOLO|10\n202610|2026-0001|Tunjangan|99|SOLO|1\n202610|9999-0001|Tunjangan|1|SOLO|1\n"
	for f, c := range map[string]string{"MGT_PAYROLL_MAIN.psv": head, "MGT_PAYROLL_DETAIL.psv": det} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reps, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.SalaryLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm=202610`); got != "0" {
		t.Errorf("periode bermasalah tetap dimuat: %s run", got)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_period WHERE pp_yymm=202610`); got != "0" {
		t.Errorf("periode bermasalah meninggalkan pyr_period: %s", got)
	}
	if len(reps[0].Exceptions) != 1 || len(reps[1].Exceptions) != 3 || !reps[0].OK() || !reps[1].OK() {
		t.Errorf("laporan periode tertahan: main exc=%d detail exc=%d", len(reps[0].Exceptions), len(reps[1].Exceptions))
	}
}

// T009e4: JAKARTA, lintas tahun (Des/Jan), audit per periode, satu transaksi, PII tidak ikut termigrasi.
func TestETL_SalaryLegacyEdges(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	for _, ym := range []string{"202601", "202602"} {
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
VALUES (1,$1,'JAKARTA','{}','x',true)`, ym); err != nil {
			t.Fatal(err)
		}
	}
	write := func(head, det string) string {
		dir := t.TempDir()
		for f, c := range map[string]string{"MGT_PAYROLL_MAIN.psv": head, "MGT_PAYROLL_DETAIL.psv": det} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	mainCols := strings.Split("PM_YYMM|PM_NIK|PM_NAMA|PM_DEPT|PM_GRADE|PM_JABATAN|PM_EMP_TYPE|PM_NPWP|PM_DATE|PM_PEMBAYARAN|PM_DATE_JOIN|PM_BASIC_PAY|PM_TUNJJABATAN|PM_TUNJRUMAH|PM_TUNJKESEJAHTERAAN|PM_TUNJTRANSPORT|PM_RATE_PERJAM|PM_TAX_CODE|PM_BANK_ACNT|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING|PM_DAY_PAY|PM_JAM_LEMBUR1|PM_JAM_LEMBUR2|PM_JAM_LEMBURNAS1|PM_JAM_LEMBURNAS2|PM_JAM_LEMBURNAS3|PM_JAM_LEMBUROFF1|PM_JAM_LEMBUROFF2|PM_JAM_LEMBUROFF3|PM_JAM_LEMBURNASP1|PM_JAM_LEMBURNASP2|PM_JAM_LEMBURNASP3|PM_DEPT_CODE|PM_REMARKS|PM_STATUS|PM_SECTION|PM_DIVISION", "|")
	detCols := strings.Split("PD_YYMM|PD_NIK|PD_NAMA|PD_NO|PD_TYPE|PD_DESCRIPTION|PD_AMOUNT|PD_STATUS|PD_TAX|PD_BASIS_AMOUNT|PD_REMARKS|PD_TAX_PAID_COMPANY|PD_DIVISION", "|")
	row := func(cols []string, kv map[string]string) string {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = kv[c]
		}
		return strings.Join(cells, "|") + "\n"
	}
	head := func(yymm, date, tun string) string {
		return row(mainCols, map[string]string{"PM_YYMM": yymm, "PM_NIK": "2026-0001", "PM_DIVISION": "JAKARTA", "PM_DATE": date,
			"PM_NPWP": "1234", "PM_BANK_ACNT": "5678", "PM_TOT_TUNJANGAN": tun, "PM_TOT_POTONGAN": "0", "PM_ROUNDING": "0", "PM_TAX_CODE": "K1", "PM_BASIC_PAY": tun})
	}
	line := func(yymm, amt string) string {
		return row(detCols, map[string]string{"PD_YYMM": yymm, "PD_NIK": "2026-0001", "PD_TYPE": "Tunjangan", "PD_NO": "1", "PD_DIVISION": "JAKARTA", "PD_AMOUNT": amt,
			"PD_DESCRIPTION": "Gaji Pokok", "PD_STATUS": "F", "PD_TAX": "2", "PD_TAX_PAID_COMPANY": "N"})
	}
	hdr := func(cols []string) string { return strings.Join(cols, "|") + "\n" }
	det := hdr(detCols) + line("202601", "10") + line("202602", "20")

	// periode ke-2 rusak (PM_DATE) → seluruh transaksi dibatalkan, periode ke-1 tidak boleh tersisa.
	bad := write(hdr(mainCols)+head("202601", "25-JAN-26", "10")+head("202602", "bukan-tanggal", "20"), det)
	if _, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: bad}, etl.SalaryLegacyOptions{CompanyID: 1}); err == nil {
		t.Fatal("tanggal rusak seharusnya menggagalkan load")
	}
	for _, q := range []string{
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_division='JAKARTA'`,
		`SELECT count(*) FROM payroll.pyr_period WHERE pp_division='JAKARTA'`,
		`SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_division='JAKARTA'`,
		`SELECT count(*) FROM payroll.pyr_migration_log WHERE pml_table='MGT_PAYROLL_MAIN' AND pml_src_rows=2`,
	} {
		if got := scalar(t, q); got != "0" {
			t.Errorf("rollback tidak bersih: %s = %s", q, got)
		}
	}

	good := write(hdr(mainCols)+head("202601", "25-JAN-26", "10")+head("202602", "25-FEB-26", "20"), det)
	reps, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: good}, etl.SalaryLegacyOptions{CompanyID: 1, ExpectMain: 2, ExpectDetail: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(reps[0].Exceptions)+len(reps[1].Exceptions) != 0 {
		t.Errorf("exception tak terduga: %v %v", reps[0].Exceptions, reps[1].Exceptions)
	}
	for q, want := range map[string]string{
		`SELECT pp_date_from||'..'||pp_date_to FROM payroll.pyr_period WHERE pp_yymm=202601 AND pp_division='JAKARTA'`:                  "2025-12-26..2026-01-25",
		`SELECT pp_date_from||'..'||pp_date_to FROM payroll.pyr_period WHERE pp_yymm=202602 AND pp_division='JAKARTA'`:                  "2026-01-26..2026-02-25",
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_division='JAKARTA' AND pr_kind='LEGACY' AND pr_status='ACTIVE'`:                  "2",
		`SELECT count(*) FROM payroll.pyr_period WHERE pp_division='JAKARTA' AND pp_status='CLOSED'`:                                    "2",
		`SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_actor='etl' AND pal_action='ETL_PERIOD_CLOSE' AND pal_division='JAKARTA'`: "2",
		`SELECT pal_after->>'tot_tunjangan' FROM payroll.pyr_audit_log WHERE pal_entity_key='202602/JAKARTA'`:                           "20.00",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// run ulang: tidak menambah audit (periode sudah CLOSED, dilewati).
	if _, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: good}, etl.SalaryLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, `SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_division='JAKARTA'`); got != "2" {
		t.Errorf("run ulang menambah audit: %s", got)
	}
	// payroll_etl hanya boleh INSERT audit ETL_*; tidak membaca, mengubah, atau menulis aksi lain.
	if _, err := etlDB.Exec(`INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action) VALUES ('etl','USER_LOGIN')`); err == nil {
		t.Error("payroll_etl menulis aksi audit non-ETL")
	}
	if _, err := etlDB.Exec(`UPDATE payroll.pyr_audit_log SET pal_actor='x'`); err == nil {
		t.Error("payroll_etl mengubah audit log")
	}
	// PM_NPWP / PM_BANK_ACNT tidak punya kolom tujuan di pyr_salary_head (PII hanya di pyr_employee, T009b).
	if got := scalar(t, `SELECT count(*) FROM information_schema.columns WHERE table_schema='payroll' AND table_name='pyr_salary_head' AND (column_name ILIKE '%npwp%' OR column_name ILIKE '%bank%')`); got != "0" {
		t.Errorf("pyr_salary_head memuat kolom PII: %s", got)
	}
}

func TestETL_AttLegacyFail(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.RunAttLegacy(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.AttLegacyOptions{CompanyID: 1}); err != nil {
		t.Fatal(err)
	}
	src := etl.PSVDir{Dir: "testdata_fail"}
	state := func() string {
		return scalar(t, `SELECT md5(string_agg(pasr_nik||(pasr_data->>'MPAR_DAY_WORK')||coalesce(pasr_premi_fail::text,'n')||coalesce(pasr_beras_fail::text,'n'), ',' ORDER BY 1)) FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='REKAP'`)
	}
	rep, err := etl.RunAttLegacyFail(ctx, etlDB, src)
	if err != nil {
		t.Fatal(err)
	}
	first := state()
	if _, err = etl.RunAttLegacyFail(ctx, etlDB, src); err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	if state() != first {
		t.Fatal("run kedua mengubah hasil (tidak idempotent)")
	}
	if rep.PremiDetail != 4 || rep.BerasDetail != 3 || rep.PremiSet != 3 || rep.PremiFailed != 1 || rep.BerasSet != 2 || rep.Updated != 3 ||
		len(rep.BerasResidue) != 1 || len(rep.NoSnapshotRow) != 1 {
		t.Errorf("laporan tak sesuai: %+v", rep)
	}
	for q, want := range map[string]string{
		`SELECT pasr_premi_fail||'/'||pasr_beras_fail FROM payroll.pyr_att_snapshot_row WHERE pasr_nik='2026-0001' AND pasr_data->>'MPAR_DAY_WORK'='24'`:             "1/2",
		`SELECT pasr_premi_fail||'/'||pasr_beras_fail FROM payroll.pyr_att_snapshot_row WHERE pasr_nik='2026-0002'`:                                                  "0/0",
		`SELECT pasr_premi_fail::text FROM payroll.pyr_att_snapshot_row WHERE pasr_nik='2026-0001' AND pasr_data->>'MPAR_DAY_WORK'='26'`:                             "0",
		`SELECT count(*) FROM payroll.pyr_att_snapshot_row WHERE pasr_kind='REKAP' AND pasr_nik='2026-0003' AND pasr_beras_fail IS NULL AND pasr_premi_fail IS NULL`: "1",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
}

// T009e5 (D-35): skip trial/202607, MAIN sintetis dari DETAIL, periode 201712 tanpa snapshot, yatim tidak menahan periode.
func TestETL_SalaryLegacyScope(t *testing.T) {
	ctx := context.Background()
	if _, err := etl.Run(ctx, etlDB, etl.PSVDir{Dir: "testdata"}, etl.Options{CompanyID: 1, ValidFrom: 202001}); err != nil {
		t.Fatal(err)
	}
	// snapshot hanya untuk 202603/SOLO; 201712/SOLO sengaja tanpa snapshot.
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
VALUES (1,202603,'SOLO','{}','x',true)`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mainCols := strings.Split("PM_YYMM|PM_NIK|PM_NAMA|PM_DEPT|PM_GRADE|PM_JABATAN|PM_EMP_TYPE|PM_DATE|PM_PEMBAYARAN|PM_DATE_JOIN|PM_BASIC_PAY|PM_TUNJJABATAN|PM_TUNJRUMAH|PM_TUNJKESEJAHTERAAN|PM_TUNJTRANSPORT|PM_RATE_PERJAM|PM_TAX_CODE|PM_TOT_TUNJANGAN|PM_TOT_POTONGAN|PM_ROUNDING|PM_DAY_PAY|PM_JAM_LEMBUR1|PM_JAM_LEMBUR2|PM_JAM_LEMBURNAS1|PM_JAM_LEMBURNAS2|PM_JAM_LEMBURNAS3|PM_JAM_LEMBUROFF1|PM_JAM_LEMBUROFF2|PM_JAM_LEMBUROFF3|PM_JAM_LEMBURNASP1|PM_JAM_LEMBURNASP2|PM_JAM_LEMBURNASP3|PM_DEPT_CODE|PM_REMARKS|PM_SECTION|PM_DIVISION", "|")
	detCols := strings.Split("PD_YYMM|PD_NIK|PD_NO|PD_TYPE|PD_DESCRIPTION|PD_AMOUNT|PD_STATUS|PD_TAX|PD_BASIS_AMOUNT|PD_REMARKS|PD_TAX_PAID_COMPANY|PD_DIVISION", "|")
	row := func(cols []string, kv map[string]string) string {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = kv[c]
		}
		return strings.Join(cells, "|") + "\n"
	}
	mrow := func(yymm, nik, div, date, tun string) string {
		return row(mainCols, map[string]string{"PM_YYMM": yymm, "PM_NIK": nik, "PM_DIVISION": div, "PM_DATE": date, "PM_TOT_TUNJANGAN": tun, "PM_TOT_POTONGAN": "0", "PM_ROUNDING": "0"})
	}
	drow := func(yymm, nik, typ, no, div, amt, desc string) string {
		return row(detCols, map[string]string{"PD_YYMM": yymm, "PD_NIK": nik, "PD_TYPE": typ, "PD_NO": no, "PD_DIVISION": div, "PD_AMOUNT": amt, "PD_DESCRIPTION": desc, "PD_STATUS": "F", "PD_TAX": "2", "PD_TAX_PAID_COMPANY": "N"})
	}
	mainRows := strings.Join(mainCols, "|") + "\n" +
		mrow("201712", "2026-0001", "SOLO", "25-DEC-17", "10") +
		mrow("202603", "2026-0001", "SOLO", "25-MAR-26", "10") +
		mrow("202512", "2026-0001", "JAKARTA", "25-DEC-25", "99") // trial: skip
	detRows := strings.Join(detCols, "|") + "\n" +
		drow("201712", "2026-0001", "Tunjangan", "1", "SOLO", "10", "Gaji Pokok") +
		drow("202603", "2026-0001", "Tunjangan", "1", "SOLO", "10", "Gaji Pokok") +
		drow("202603", "2026-0009", "Tunjangan", "1", "SOLO", "100.005", "Gaji Pokok") + // yatim: tanpa MAIN
		drow("202603", "2026-0009", "Potongan", "32", "SOLO", "1.234", "Pinjaman Pokok") +
		drow("202607", "2026-0001", "Tunjangan", "1", "SOLO", "5", "Gaji Pokok") + // SOLO 202607: skip
		drow("202512", "2026-0001", "Tunjangan", "1", "JAKARTA", "99", "Gaji Pokok") // trial: skip
	for f, c := range map[string]string{"MGT_PAYROLL_MAIN.psv": mainRows, "MGT_PAYROLL_DETAIL.psv": detRows} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reps, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.SalaryLegacyOptions{CompanyID: 1, ExpectMain: 3, ExpectDetail: 6})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		if !r.OK() || len(r.Exceptions) != 0 {
			t.Errorf("%s src=%d dst=%d skip=%d exc=%v", r.Table, r.SrcRows, r.DstRows, r.SkippedRows, r.Exceptions)
		}
	}
	if reps[0].SkippedRows != 1 || reps[1].SkippedRows != 2 || len(reps[1].Skipped) != 2 {
		t.Errorf("skip main=%d detail=%d (%v)", reps[0].SkippedRows, reps[1].SkippedRows, reps[1].Skipped)
	}
	if len(reps[0].Warnings) != 2 { // MAIN sintetis + EMP tidak ditemukan
		t.Errorf("warning MAIN sintetis = %v, want 2", reps[0].Warnings)
	}
	for q, want := range map[string]string{
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_kind='LEGACY' AND pr_yymm IN (201712,202603,202512,202607)`:                  "2",
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm=201712 AND pr_snapshot_id IS NULL`:                                      "1",
		`SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm=202603 AND pr_snapshot_id IS NOT NULL`:                                  "1",
		`SELECT count(*) FROM payroll.pyr_period WHERE pp_yymm IN (202512,202607)`:                                                  "0",
		`SELECT pp_date_from||'..'||pp_date_to FROM payroll.pyr_period WHERE pp_yymm=201712`:                                        "2017-11-26..2017-12-25",
		`SELECT pr_emp_count||'/'||pr_line_count FROM payroll.pyr_run WHERE pr_yymm=202603`:                                         "2/3",
		`SELECT psh_tot_tunjangan||'/'||psh_tot_potongan||'/'||psh_rounding FROM payroll.pyr_salary_head WHERE psh_nik='2026-0009'`: "100.01/1.23/0.00",
	} {
		if got := scalar(t, q); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	// run ulang idempotent, hitungan tetap seimbang
	reps2, err := etl.RunSalaryLegacy(ctx, etlDB, etl.PSVDir{Dir: dir}, etl.SalaryLegacyOptions{CompanyID: 1})
	if err != nil {
		t.Fatalf("run ulang: %v", err)
	}
	for _, r := range reps2 {
		if !r.OK() {
			t.Errorf("run ulang %s src=%d dst=%d skip=%d", r.Table, r.SrcRows, r.DstRows, r.SkippedRows)
		}
	}
}

// TestETL_PruneExcluded: run LEGACY sisa muatan lama di periode yang kini di-SKIP (D-35.1) dihapus; dry-run tidak
// mengubah apa pun; periode dalam cakupan tidak tersentuh; run non-LEGACY membatalkan prune.
func TestETL_PruneExcluded(t *testing.T) {
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
		 VALUES (1,202401,'JAKARTA','2023-12-26','2024-01-25','OPEN'),(1,202612,'JAKARTA','2026-11-26','2026-12-25','OPEN')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_kind, pr_status, pr_rule_set_id, pr_pay_date)
		 VALUES (1,202401,'JAKARTA','LEGACY','ACTIVE',0,'2024-01-25'),(1,202612,'JAKARTA','LEGACY','ACTIVE',0,'2026-12-25')`,
		`UPDATE payroll.pyr_period p SET pp_status='CLOSED', pp_active_run_id=(SELECT pr_id FROM payroll.pyr_run r
		 WHERE r.pr_yymm=p.pp_yymm AND r.pr_division=p.pp_division AND r.pr_company_id=p.pp_company_id)
		 WHERE pp_yymm IN (202401,202612) AND pp_division='JAKARTA'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	count := func() string {
		return scalar(t, `SELECT (SELECT count(*) FROM payroll.pyr_run WHERE pr_yymm IN (202401,202612) AND pr_division='JAKARTA')||'/'||
		  (SELECT count(*) FROM payroll.pyr_period WHERE pp_yymm IN (202401,202612) AND pp_division='JAKARTA')`)
	}
	runs, err := etl.PruneExcluded(ctx, ownDB, etl.PruneOptions{CompanyID: 1})
	if err != nil || len(runs) != 1 || runs[0].YYMM != "202401" {
		t.Fatalf("dry-run runs=%v err=%v", runs, err)
	}
	if got := count(); got != "2/2" {
		t.Fatalf("dry-run mengubah data: %s", got)
	}
	if _, err = etl.PruneExcluded(ctx, ownDB, etl.PruneOptions{CompanyID: 1, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != "1/1" {
		t.Errorf("setelah prune run/periode = %s, want 1/1 (202612 harus tetap)", got)
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_action='ETL_PRUNE_EXCLUDED' AND pal_entity_key='202401/JAKARTA'`); n != "1" {
		t.Errorf("audit prune = %s, want 1", n)
	}
	// idempotent
	if runs, err = etl.PruneExcluded(ctx, ownDB, etl.PruneOptions{CompanyID: 1, Apply: true}); err != nil || len(runs) != 0 {
		t.Errorf("run ulang runs=%v err=%v", runs, err)
	}
}
