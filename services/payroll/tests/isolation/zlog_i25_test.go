package isolation

// I25 (09§6.5): log of sync / run commit / master RPC holds no employee name or salary amount.
// Everything runs against the shared testcontainers DB with the zerolog global logger redirected to a
// buffer at DEBUG, plus the combined stdout/stderr of the real cmd/sync binary. The file name sorts
// last on purpose: the ETL fixtures it loads stay in the DB afterwards.

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const (
	i25Period  = 202611
	i25Name    = "Rahasia Sembilan"
	i25RPCName = "Nama Rahasia Tujuh"
	i25Salary  = "8765432.10"
	i25Key     = "i25-test-enc-key-0123456789abcdef"
	i25HMAC    = "i25-test-hmac-key-0123456789abcdef"
)

// i25Secrets are literal values that must never reach a log: employee names (seed, ETL fixture,
// I25 fixtures), salary amounts (ETL fixture, I25 fixture; plain and thousand-separated) and raw PII.
var i25Secrets = []string{
	"Solo One", "Jkt One", "Other One", i25Name, i25RPCName,
	"Budi", "Siti", "Andi", "Joko", "Sari",
	"5000000", "5.000.000", "5,000,000", "5500000", "4000000", "5100000", "8765432", "8.765.432", "8,765,432",
	"1234567890", "3372010101900001", "12.345.678.9-012.000", rawI25NPWP, rawI25KTP, rawI25Bank,
}

const (
	rawI25NPWP = "01.234.567.8-901.000"
	rawI25KTP  = "3372010101010001"
	rawI25Bank = "9988776655"
)

// i25Scan returns every secret found in out (word-bounded so "Sari" does not match inside a longer word).
func i25Scan(out string) []string {
	var hits []string
	for _, s := range i25Secrets {
		if regexp.MustCompile(`(^|[^A-Za-z0-9.,])` + regexp.QuoteMeta(s) + `($|[^A-Za-z0-9])`).MatchString(out) {
			hits = append(hits, s)
		}
	}
	return hits
}

func TestI25_ScannerSelfCheck(t *testing.T) {
	if h := i25Scan(`{"level":"info","msg":"nik=1001 emp=Budi total=5000000"}`); len(h) != 2 {
		t.Fatalf("scanner must catch a name and an amount, got %v", h)
	}
	if h := i25Scan(`{"level":"info","time":"2026-11-04T10:00:00Z","rows":2274418,"msg":"Sarian ok"}`); len(h) != 0 {
		t.Fatalf("scanner false positive: %v", h)
	}
}

// syncBuf is a goroutine-safe log sink.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type i25Rules struct{}

func (i25Rules) ID() int                                 { return 1 }
func (i25Rules) LegacyCompat() bool                      { return true }
func (i25Rules) Constant(string) (decimal.Decimal, bool) { return decimal.Decimal{}, false }

// i25Calc turns each employee component into one line with its amount (the commit path under test is
// the service, not the formulas).
func i25Calc(in engine.Input) (engine.Result, error) {
	res := engine.Result{NIK: in.Emp.NIK}
	for _, c := range in.Components {
		a := c.Amount
		res.Lines = append(res.Lines, engine.Line{Key: c.Key, Amount: &a})
	}
	return res, nil
}

// runSync executes the real cmd/sync binary and returns its combined output.
func runSync(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	// "employees" reconciles source vs destination row counts per division; the shared seed employees
	// (S1, J1, O1) make that check fail by design here. The data is loaded and the output is scanned
	// either way, so only that subcommand may exit non-zero.
	if err != nil && args[0] != "employees" {
		t.Fatalf("sync %s: %v\n%s", args[0], err, out)
	}
	return string(out)
}

func buildSync(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sync")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/sync")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/sync: %v\n%s", err, out)
	}
	return bin
}

func i25Seed(t *testing.T) {
	t.Helper()
	// The run API resolves the rule set itself (D-53): it needs an ACTIVE one (the RLS seed leaves it DRAFT).
	var prevStatus string
	if err := ownDB.QueryRow(`SELECT prs_status FROM payroll.pyr_rule_set WHERE prs_id=1`).Scan(&prevStatus); err != nil {
		t.Fatalf("read rule set: %v", err)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_rule_set SET prs_status='ACTIVE' WHERE prs_id=1`); err != nil {
		t.Fatalf("activate rule set: %v", err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`UPDATE payroll.pyr_rule_set SET prs_status='` + prevStatus + `' WHERE prs_id=1`,
			`UPDATE payroll.pyr_period SET pp_status='OPEN', pp_active_run_id=NULL WHERE pp_company_id=1 AND pp_yymm=` + fmt.Sprint(i25Period),
			`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=` + fmt.Sprint(i25Period),
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=` + fmt.Sprint(i25Period),
			`DELETE FROM payroll.pyr_att_snapshot WHERE pas_checksum='i25'`,
			`DELETE FROM payroll.pyr_attd_days WHERE pad_yymm=` + fmt.Sprint(i25Period),
			`DELETE FROM payroll.pyr_employee_component WHERE pec_nik LIKE 'I25-%'`,
			`DELETE FROM payroll.pyr_employee_nik_hist WHERE penh_nik LIKE 'I25-%'`,
			`DELETE FROM payroll.pyr_employee WHERE pe_nik LIKE 'I25-%'`,
		} {
			if _, err := ownDB.Exec(q); err != nil {
				t.Errorf("cleanup: %v\n%s", err, q)
			}
		}
	})
	for _, q := range []string{
		`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_dept)
		 VALUES (1,'I25-1','SOLO','` + i25Name + `','Staff','TK',2,'I25-1','PROD')`,
		`INSERT INTO payroll.pyr_employee_component (pec_company_id, pec_nik, pec_division, pec_component_sysid, pec_sts, pec_tax, pec_amount, pec_frz_flag)
		 SELECT 1,'I25-1','SOLO',c.pc_sysid,'F',2,` + i25Salary + `,2 FROM payroll.pyr_component c WHERE c.pc_company_id=1 AND c.pc_code='T001'`,
		`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,` + fmt.Sprint(i25Period) + `,'SOLO',26)`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
		 VALUES (1,` + fmt.Sprint(i25Period) + `,'SOLO','{1}','i25',true)`,
		`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		 SELECT pas_snapshot_id, 'REKAP', 'I25-1', '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb FROM payroll.pyr_att_snapshot WHERE pas_checksum='i25'`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
}

func TestI25_LogsHoldNoNameOrSalary(t *testing.T) {
	buf := &syncBuf{}
	old := zlog.Logger
	zlog.Logger = zerolog.New(buf).Level(zerolog.DebugLevel).With().Timestamp().Logger()
	t.Cleanup(func() { zlog.Logger = old })
	zlog.Info().Msg("i25 canary")

	// The inputs ETL reconciles pyr_rule_constant row counts; park the seed row (RLS tests) meanwhile.
	if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_rule_constant WHERE prc_key='GLOBAL'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_rule_constant (prc_company_id, prc_rule_set_id, prc_key, prc_value, prc_division) VALUES (1,1,'GLOBAL',1,NULL)`); err != nil {
			t.Errorf("restore rule constant: %v", err)
		}
	})

	// 1. cmd/sync (real binary, role payroll_etl): master, employees, inputs, attendance, salary.
	bin := buildSync(t)
	dir, err := filepath.Abs("../etl/testdata")
	if err != nil {
		t.Fatal(err)
	}
	dsn := fmt.Sprintf("host=%s port=%s user=payroll_etl password=payroll_etl123 dbname=payroll sslmode=disable", pgHost, pgPort)
	env := []string{"PAYROLL_PII_KEY=" + i25Key, "PAYROLL_PII_HMAC_KEY=" + i25HMAC}
	var syncOut strings.Builder
	for _, a := range [][]string{
		{"migrate", "-dir", dir, "-dsn", dsn},
		{"inputs", "-dir", dir, "-dsn", dsn},
		{"att-legacy", "-dir", dir, "-dsn", dsn},
		{"salary-legacy", "-dir", dir, "-dsn", dsn, "-allow-exceptions"},
		{"employees", "-dir", dir, "-dsn", dsn},
	} {
		syncOut.WriteString(runSync(t, bin, env, a...))
	}
	if !strings.Contains(syncOut.String(), "sha256=") {
		t.Fatalf("sync produced no report line; output: %.300s", syncOut.String())
	}

	// 2. Commit run in the service (scoped tx, audit row, loader, persist).
	i25Seed(t)
	ctx := ctxFor(t, userSolo)
	svc := run.NewServiceWith(appDB, func(context.Context, *sql.Tx, run.Query, int) (run.Rules, error) {
		return run.Rules{Set: i25Rules{}}, nil
	}, audit.NewWriter(), run.Runner{Calc: i25Calc})
	res, err := svc.Commit(ctx, run.CommitCommand{CompanyID: 1, Division: "SOLO", YYMM: i25Period, RuleSetID: 1})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if res.Summary.EmpCount == 0 || res.Summary.TotalT.IsZero() {
		t.Fatalf("commit produced nothing: %+v", res.Summary)
	}

	// 3. Master RPC with the PII-revealing permission: create, get, list, update, plus error paths.
	h := grpcdelivery.NewMasterHandler(master.NewService(appDB, postgres.NewMasterRepository(), audit.NewWriter(), i25Key, i25HMAC).
		WithComponents(postgres.NewComponentRepository()).WithAttachments(postgres.NewAttachmentRepository()))
	pctx := context.WithValue(ctx, grpcdelivery.AuthPermissionsKey, []string{"payroll.master.employee.view",
		"payroll.master.employee.create", "payroll.master.employee.update"})
	in := &payrollv1.EmployeeInput{Nama: i25RPCName, StsPayroll: 2, OrionSubAcnt: "I25-R", Level: "STAFF", DateJoin: "2026-01-05",
		Npwp: rawI25NPWP, Ktp: rawI25KTP, BankAcnt: rawI25Bank}
	created, err := h.CreateEmployee(pctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "I25-RPC", Input: in})
	if err != nil {
		t.Fatalf("rpc create: %v", err)
	}
	if _, err := h.GetEmployee(pctx, &payrollv1.GetEmployeeRequest{Nik: "I25-RPC"}); err != nil {
		t.Fatalf("rpc get: %v", err)
	}
	if _, err := h.ListEmployees(pctx, &payrollv1.ListEmployeesRequest{Q: "Rahasia", Limit: 10}); err != nil {
		t.Fatalf("rpc list: %v", err)
	}
	if _, err := h.UpdateEmployee(pctx, &payrollv1.UpdateEmployeeRequest{Id: created.GetData().GetId(), Input: in, IfMatch: created.GetData().GetEtag()}); err != nil {
		t.Fatalf("rpc update: %v", err)
	}
	// error paths must not echo the payload either (duplicate NIK, stale etag, out of scope).
	_, _ = h.CreateEmployee(pctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "I25-RPC", Input: in})  //nolint:errcheck // expected failure
	_, _ = h.UpdateEmployee(pctx, &payrollv1.UpdateEmployeeRequest{Id: created.GetData().GetId(), Input: in, IfMatch: "999"})   //nolint:errcheck // expected failure
	_, _ = h.CreateEmployee(pctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "JAKARTA", Nik: "I25-X", Input: in}) //nolint:errcheck // expected failure

	// 4. Run API with JWT-style context: dry, commit, get, list, lines (full and masked), period list, plus error paths.
	rh := newRunHandler()
	full := ctxPerms(t, userSolo, permsRunAll...)
	blind := ctxPerms(t, userSolo, permsRunNoAmount...)
	if _, err := rh.CreateRun(full, runReq("SOLO", payrollv1.RunMode_RUN_MODE_DRY)); err != nil {
		t.Fatalf("api dry: %v", err)
	}
	// Successful commit through the handler (reprocess of the period committed in step 2: supersedes it).
	cm, err := rh.CreateRun(full, runReq("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Fatalf("api commit: %v", err)
	}
	runID := cm.GetData().GetRunId()
	if runID == 0 || runID == res.RunID {
		t.Fatalf("api commit run id = %d (service commit %d)", runID, res.RunID)
	}
	for _, c := range []context.Context{full, blind} {
		if _, err := rh.GetRun(c, &payrollv1.GetRunRequest{RunId: runID}); err != nil {
			t.Fatalf("api get: %v", err)
		}
		if _, err := rh.ListRunLines(c, &payrollv1.ListRunLinesRequest{RunId: runID}); err != nil {
			t.Fatalf("api lines: %v", err)
		}
		if _, err := rh.ListRuns(c, &payrollv1.ListRunsRequest{Division: "SOLO"}); err != nil {
			t.Fatalf("api list: %v", err)
		}
		if _, err := rh.ListPeriods(c, &payrollv1.ListPeriodsRequest{Division: "SOLO"}); err != nil {
			t.Fatalf("api periods: %v", err)
		}
	}
	_, err = rh.CreateRun(full, runReq("JAKARTA", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "api commit JAKARTA", err, apperr.CodeDivisionForbidden)
	if _, err := rh.ApprovePeriod(full, &payrollv1.ApprovePeriodRequest{Yymm: i25Period, Division: "SOLO"}); err != nil {
		t.Fatalf("api approve: %v", err)
	}
	if _, err := rh.ClosePeriod(full, &payrollv1.ClosePeriodRequest{Yymm: i25Period, Division: "SOLO"}); err != nil {
		t.Fatalf("api close: %v", err)
	}
	var st string
	if err := ownDB.QueryRow(`SELECT pp_status FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=$1 AND pp_division='SOLO'`, i25Period).Scan(&st); err != nil || st != "CLOSED" {
		t.Fatalf("period status = %q (%v), want CLOSED", st, err)
	}
	// a commit into the CLOSED period is refused (error path, logs must stay clean too).
	_, err = rh.CreateRun(full, runReq("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err == nil {
		t.Fatal("commit into CLOSED period succeeded")
	}

	logs := buf.String()
	if !strings.Contains(logs, "i25 canary") {
		t.Fatal("log capture is not working")
	}
	t.Logf("scanned %d bytes of in-process log and %d bytes of sync output", len(logs), syncOut.Len())
	if hits := i25Scan(logs + "\n" + syncOut.String()); len(hits) != 0 {
		t.Errorf("I25: %d secret(s) in log output: %v\n--- in-process log ---\n%s\n--- sync output ---\n%s", len(hits), hits, logs, syncOut.String())
	}
}
