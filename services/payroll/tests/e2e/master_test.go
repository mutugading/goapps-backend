// Package e2e holds end-to-end tests of the payroll gRPC handlers against a real
// PostgreSQL 18 (testcontainers), connecting as payroll_app like the service.
// Gated by INTEGRATION_TEST=true (needs Docker); `make test-e2e` sets it.
package e2e

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

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	app "github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

const (
	encKey   = "test-enc-key-0123456789-0123456789"
	hmacKey  = "test-hmac-key-0123456789-012345678"
	userRW   = "user-rw"   // SOLO scope, may update (sees PII)
	userRO   = "user-ro"   // SOLO scope, view only (PII masked)
	userJkt  = "user-jkt"  // JAKARTA scope only
	userBoth = "user-both" // SOLO + JAKARTA
	rawNPWP  = "01.234.567.8-901.000"
	rawKTP   = "3372010101010001"
	rawBank  = "1234567890"
)

var (
	appDB   *postgres.DB
	ownDB   *sql.DB
	handler *grpcdelivery.MasterHandler
)

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("skip e2e tests: INTEGRATION_TEST != true")
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
	root, err := filepath.Abs("../../../../..")
	if err != nil {
		return nil, err
	}
	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("postgres"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath: filepath.Join(root, "deploy", "payroll", "bootstrap.sql"), ContainerFilePath: "/bootstrap/bootstrap.sql", FileMode: 0o644,
		}),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	cleanup := func() { _ = testcontainers.TerminateContainer(ctr) } //nolint:errcheck // best effort
	fail := func(err error) (func(), error) { cleanup(); return nil, err }

	code, _, err := ctr.Exec(ctx, []string{"psql", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1",
		"-v", "pw_app=payroll_app123", "-v", "pw_etl=payroll_etl123", "-v", "pw_ro=payroll_ro123",
		"-v", "pw_mig=payroll_mig123", "-f", "/bootstrap/bootstrap.sql"})
	if err != nil || code != 0 {
		return fail(fmt.Errorf("bootstrap: code=%d err=%v", code, err))
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		return fail(err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return fail(err)
	}
	dsn := func(u, pw string) string {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=payroll sslmode=disable", host, port.Port(), u, pw)
	}
	if ownDB, err = sql.Open("pgx", dsn("payroll_migrator", "payroll_mig123")); err != nil {
		return fail(err)
	}
	files, _ := filepath.Glob("../../migrations/postgres/*.up.sql") //nolint:errcheck // pattern is valid
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f) //nolint:gosec // repo migrations
		if err != nil {
			return fail(err)
		}
		if _, err := ownDB.ExecContext(ctx, string(body)); err != nil {
			return fail(fmt.Errorf("migration %s: %w", filepath.Base(f), err))
		}
	}
	for _, s := range []string{
		`INSERT INTO payroll.pyr_currency (pcy_code, pcy_minor_unit, pcy_takehome_round) VALUES ('IDR', 2, -2)`,
		`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('MGT', 'Mutu Gading')`,
		`INSERT INTO payroll.pyr_division_ref VALUES (1,'SOLO','Solo'), (1,'JAKARTA','Jakarta')`,
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_company_id, pus_division) VALUES
		   ('` + userRW + `',1,'SOLO'), ('` + userRO + `',1,'SOLO'), ('` + userJkt + `',1,'JAKARTA'), ('` + userBoth + `',1,'SOLO'), ('` + userBoth + `',1,'JAKARTA')`,
	} {
		if _, err := ownDB.ExecContext(ctx, s); err != nil {
			return fail(fmt.Errorf("seed: %w", err))
		}
	}
	rawApp, err := sql.Open("pgx", dsn("payroll_app", "payroll_app123"))
	if err != nil {
		return fail(err)
	}
	appDB = postgres.NewDBFromSQL(rawApp)
	handler = grpcdelivery.NewMasterHandler(app.NewService(appDB, postgres.NewMasterRepository(), auditinfra.NewWriter(), encKey, hmacKey))
	return cleanup, nil
}

// ctxFor builds a request context as the auth interceptor would: identity with
// resolved scopes plus the JWT permissions.
func ctxFor(t *testing.T, userID string, perms ...string) context.Context {
	t.Helper()
	ctx := context.Background()
	scopes, err := postgres.NewScopeResolver(appDB).Resolve(ctx, userID)
	if err != nil {
		t.Fatalf("resolve %s: %v", userID, err)
	}
	ctx = scope.WithIdentity(ctx, scope.Identity{UserID: userID, Scopes: scopes})
	return context.WithValue(ctx, grpcdelivery.AuthPermissionsKey, perms)
}

var (
	permsRW = []string{"payroll.master.employee.view", "payroll.master.employee.create", "payroll.master.employee.update",
		"payroll.master.company.view", "payroll.master.company.create", "payroll.master.company.update",
		"payroll.master.currency.view", "payroll.master.currency.create", "payroll.master.currency.update"}
	permsRO = []string{"payroll.master.employee.view", "payroll.master.company.view", "payroll.master.currency.view"}
)

func input(nama string) *payrollv1.EmployeeInput {
	return &payrollv1.EmployeeInput{
		Nama: nama, StsPayroll: 2, OrionSubAcnt: "ORI-1", CutiTahunan: "12.25",
		Npwp: rawNPWP, Ktp: rawKTP, BankAcnt: rawBank, Level: "STAFF", DateJoin: "2026-01-05",
	}
}

func mustCreate(t *testing.T, nik, nama string) *payrollv1.Employee {
	t.Helper()
	resp, err := handler.CreateEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.CreateEmployeeRequest{
		CompanyId: 1, Division: "SOLO", Nik: nik, Input: input(nama),
	})
	if err != nil {
		t.Fatalf("create %s: %v", nik, err)
	}
	return resp.GetData()
}

func code(t *testing.T, err error) apperr.Code {
	t.Helper()
	c, ok := grpcdelivery.CodeFromError(err)
	if !ok {
		t.Fatalf("no payroll code in error: %v", err)
	}
	return c
}

func TestEmployeeCreateGetListUpdate(t *testing.T) {
	e := mustCreate(t, "E100", "Budi Santoso")
	if e.GetEtag() != "1" || e.GetId() == 0 || e.GetPayCurrency() != "IDR" {
		t.Fatalf("unexpected created row: %+v", e)
	}
	if e.GetNpwp() != rawNPWP || e.GetCutiTahunan() != "12.3" { // half-up to numeric(5,1)
		t.Fatalf("npwp=%q cuti=%q", e.GetNpwp(), e.GetCutiTahunan())
	}

	got, err := handler.GetEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.GetEmployeeRequest{Nik: "E100"})
	if err != nil || got.GetData().GetNama() != "Budi Santoso" {
		t.Fatalf("get: %v %+v", err, got)
	}

	list, err := handler.ListEmployees(ctxFor(t, userRW, permsRW...), &payrollv1.ListEmployeesRequest{Q: "budi", Limit: 10})
	if err != nil || len(list.GetData()) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}

	in := input("Budi S. Updated")
	in.Npwp, in.Ktp, in.BankAcnt = "", "", "" // empty PII keeps stored ciphertext
	upd, err := handler.UpdateEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.UpdateEmployeeRequest{Id: e.GetId(), Input: in, IfMatch: e.GetEtag()})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.GetData().GetEtag() != "2" || upd.GetData().GetNama() != "Budi S. Updated" {
		t.Fatalf("update result: %+v", upd.GetData())
	}
	if upd.GetData().GetNpwp() != rawNPWP || upd.GetData().GetKtp() != rawKTP {
		t.Fatal("empty PII on update must keep the stored value")
	}
}

func TestUpdateStaleIfMatchIs412(t *testing.T) {
	e := mustCreate(t, "E200", "Siti")
	ctx := ctxFor(t, userRW, permsRW...)
	if _, err := handler.UpdateEmployee(ctx, &payrollv1.UpdateEmployeeRequest{Id: e.GetId(), Input: input("Siti A"), IfMatch: "1"}); err != nil {
		t.Fatalf("first update: %v", err)
	}
	_, err := handler.UpdateEmployee(ctx, &payrollv1.UpdateEmployeeRequest{Id: e.GetId(), Input: input("Siti B"), IfMatch: "1"})
	if err == nil || code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("want VERSION_MISMATCH, got %v", err)
	}
	if got := apperr.CodeVersionMismatch.HTTPStatus(); got != 412 {
		t.Fatalf("VERSION_MISMATCH must map to HTTP 412, got %d", got)
	}
	// Row must be untouched by the rejected write.
	cur, _ := handler.GetEmployee(ctx, &payrollv1.GetEmployeeRequest{Nik: "E200"}) //nolint:errcheck // asserted below
	if cur.GetData().GetNama() != "Siti A" || cur.GetData().GetEtag() != "2" {
		t.Fatalf("stale write leaked: %+v", cur.GetData())
	}
}

func TestPIIEncryptedAtRestAndMaskedWithoutPermission(t *testing.T) {
	e := mustCreate(t, "E300", "Andi")
	var npwp, ktp, bank []byte
	var hash []byte
	if err := ownDB.QueryRow(`SELECT pe_npwp_enc, pe_ktp_enc, pe_bank_acnt_enc, pe_npwp_hash FROM payroll.pyr_employee WHERE pe_id=$1`, e.GetId()).
		Scan(&npwp, &ktp, &bank, &hash); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"npwp": npwp, "ktp": ktp, "bank": bank} {
		if len(b) == 0 || strings.Contains(string(b), rawNPWP) || strings.Contains(string(b), rawKTP) || strings.Contains(string(b), rawBank) {
			t.Fatalf("%s not encrypted at rest", name)
		}
	}
	if len(hash) != 32 {
		t.Fatalf("npwp hash length %d", len(hash))
	}

	got, err := handler.GetEmployee(ctxFor(t, userRO, permsRO...), &payrollv1.GetEmployeeRequest{Nik: "E300"})
	if err != nil {
		t.Fatal(err)
	}
	d := got.GetData()
	if d.GetNpwp() != "***" || d.GetKtp() != "***" || d.GetBankAcnt() != "***" {
		t.Fatalf("PII must be masked for view-only caller: %q %q %q", d.GetNpwp(), d.GetKtp(), d.GetBankAcnt())
	}

	// Audit must never contain plaintext PII.
	var after string
	if err := ownDB.QueryRow(`SELECT pal_after::text FROM payroll.pyr_audit_log WHERE pal_entity='pyr_employee' AND pal_entity_key=$1 AND pal_action='CREATE'`,
		fmt.Sprint(e.GetId())).Scan(&after); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{rawNPWP, rawKTP, rawBank} {
		if strings.Contains(after, raw) {
			t.Fatalf("audit leaks PII %q", raw)
		}
	}
}

func TestScopeIsolationAndOldNIKResolver(t *testing.T) {
	e := mustCreate(t, "E400", "Rina")
	// Another division's user cannot see the row.
	_, err := handler.GetEmployee(ctxFor(t, userJkt, permsRO...), &payrollv1.GetEmployeeRequest{Nik: "E400"})
	if err == nil || code(t, err) != apperr.CodeNotFound {
		t.Fatalf("want NOT_FOUND across scope, got %v", err)
	}
	// Creating outside scope is forbidden.
	_, err = handler.CreateEmployee(ctxFor(t, userJkt, permsRW...), &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "E401", Input: input("X")})
	if err == nil || code(t, err) != apperr.CodeDivisionForbidden {
		t.Fatalf("want DIVISION_FORBIDDEN, got %v", err)
	}
	// An old NIK recorded in history resolves to the same employee (13 3.1).
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_employee_nik_hist (penh_employee_id, penh_nik, penh_valid_from, penh_source) VALUES ($1,'OLD400','2020-01-01','HRIS')`, e.GetId()); err != nil {
		t.Fatal(err)
	}
	got, err := handler.GetEmployee(ctxFor(t, userRO, permsRO...), &payrollv1.GetEmployeeRequest{Nik: "OLD400"})
	if err != nil || got.GetData().GetId() != e.GetId() {
		t.Fatalf("old NIK resolve: %v %+v", err, got)
	}
}

func TestDuplicateNIKIsConflict(t *testing.T) {
	mustCreate(t, "E500", "Dewi")
	_, err := handler.CreateEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "E500", Input: input("Dewi 2")})
	if err == nil || code(t, err) != apperr.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
}

func TestCompanyDivisionCurrencyETag(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	cur, err := handler.CreateCurrency(ctx, &payrollv1.CreateCurrencyRequest{Code: "USD", MinorUnit: 2, TakehomeRound: 2})
	if err != nil {
		t.Fatalf("create currency: %v", err)
	}
	if _, err := handler.UpdateCurrency(ctx, &payrollv1.UpdateCurrencyRequest{Code: "USD", MinorUnit: 2, TakehomeRound: 1, IfMatch: cur.GetData().GetEtag()}); err != nil {
		t.Fatalf("update currency: %v", err)
	}
	if _, err := handler.UpdateCurrency(ctx, &payrollv1.UpdateCurrencyRequest{Code: "USD", MinorUnit: 2, TakehomeRound: 0, IfMatch: "1"}); err == nil || code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("stale currency update: %v", err)
	}

	co, err := handler.GetCompany(ctx, &payrollv1.GetCompanyRequest{CompanyId: 1})
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	up, err := handler.UpdateCompany(ctx, &payrollv1.UpdateCompanyRequest{CompanyId: 1, Name: "Mutu Gading Tbk", Npwp: rawNPWP, Active: true, IfMatch: co.GetData().GetEtag()})
	if err != nil || up.GetData().GetNpwp() != rawNPWP {
		t.Fatalf("update company: %v %+v", err, up)
	}
	if _, err := handler.UpdateCompany(ctx, &payrollv1.UpdateCompanyRequest{CompanyId: 1, Name: "X", Active: true, IfMatch: co.GetData().GetEtag()}); err == nil || code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("stale company update: %v", err)
	}
	roCo, _ := handler.GetCompany(ctxFor(t, userRO, permsRO...), &payrollv1.GetCompanyRequest{CompanyId: 1}) //nolint:errcheck // asserted below
	if roCo.GetData().GetNpwp() != "***" {
		t.Fatalf("company NPWP must be masked, got %q", roCo.GetData().GetNpwp())
	}

	nc, err := handler.CreateCompany(ctx, &payrollv1.CreateCompanyRequest{Code: "ABC", Name: "ABC Co", Npwp: rawNPWP})
	if err != nil || nc.GetData().GetBaseCurrency() != "IDR" || nc.GetData().GetEtag() != "1" {
		t.Fatalf("create company: %v %+v", err, nc)
	}

	div, err := handler.CreateDivision(ctx, &payrollv1.CreateDivisionRequest{CompanyId: 1, Code: "BANDUNG", Name: "Bandung"})
	if err != nil || div.GetData().GetEtag() != "1" {
		t.Fatalf("create division: %v", err)
	}
	if _, err := handler.UpdateDivision(ctx, &payrollv1.UpdateDivisionRequest{CompanyId: 1, Code: "BANDUNG", Name: "Bdg", IfMatch: "7"}); err == nil || code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("stale division update: %v", err)
	}
}

func TestAmbiguousNIKNeedsDivision(t *testing.T) {
	mustCreate(t, "E600", "Solo Orang")
	if _, err := handler.CreateEmployee(ctxFor(t, userJkt, permsRW...), &payrollv1.CreateEmployeeRequest{
		CompanyId: 1, Division: "JAKARTA", Nik: "E600", Input: input("Jakarta Orang")}); err != nil {
		t.Fatalf("same NIK in another division must be allowed: %v", err)
	}
	both := ctxFor(t, userBoth, permsRW...)
	if _, err := handler.GetEmployee(both, &payrollv1.GetEmployeeRequest{Nik: "E600"}); err == nil || code(t, err) != apperr.CodeConflict {
		t.Fatalf("want CONFLICT for ambiguous NIK, got %v", err)
	}
	got, err := handler.GetEmployee(both, &payrollv1.GetEmployeeRequest{Nik: "E600", Division: "JAKARTA"})
	if err != nil || got.GetData().GetNama() != "Jakarta Orang" {
		t.Fatalf("division disambiguation: %v %+v", err, got)
	}
}

func TestGlobalMasterScopeForbidden(t *testing.T) {
	var other int16
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('OTH','Other') RETURNING pco_company_id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	ctx := ctxFor(t, userRW, permsRW...)
	check := func(name string, err error) {
		t.Helper()
		if err == nil || code(t, err) != apperr.CodeDivisionForbidden {
			t.Fatalf("%s: want DIVISION_FORBIDDEN, got %v", name, err)
		}
	}
	_, err := handler.GetCompany(ctx, &payrollv1.GetCompanyRequest{CompanyId: int32(other)})
	check("get company", err)
	_, err = handler.UpdateCompany(ctx, &payrollv1.UpdateCompanyRequest{CompanyId: int32(other), Name: "X", Active: true, IfMatch: "1"})
	check("update company", err)
	_, err = handler.CreateDivision(ctx, &payrollv1.CreateDivisionRequest{CompanyId: int32(other), Code: "D1", Name: "D1"})
	check("create division", err)
	_, err = handler.ListDivisions(ctx, &payrollv1.ListDivisionsRequest{CompanyId: int32(other)})
	check("list divisions", err)
	list, err := handler.ListCompanies(ctx, &payrollv1.ListCompaniesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list.GetData() {
		if int16(c.GetCompanyId()) == other {
			t.Fatal("ListCompanies leaked a company outside scope")
		}
	}
}

func TestCurrencyAuditAndInputGuards(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	if _, err := handler.CreateCurrency(ctx, &payrollv1.CreateCurrencyRequest{Code: "EUR", MinorUnit: 2, TakehomeRound: 0}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ownDB.QueryRow(`SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_entity='pyr_currency' AND pal_entity_key='EUR' AND pal_company_id=1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("currency audit row under caller company: n=%d err=%v", n, err)
	}

	in := input("Mask Echo")
	in.Npwp = "***"
	_, err := handler.CreateEmployee(ctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "E700", Input: in})
	if err == nil || code(t, err) != apperr.CodeValidation {
		t.Fatalf("masked PII input must be rejected, got %v", err)
	}
	if _, err := handler.UpdateCompany(ctx, &payrollv1.UpdateCompanyRequest{CompanyId: 1, Name: "X", Npwp: "***", Active: true, IfMatch: "1"}); err == nil || code(t, err) != apperr.CodeValidation {
		t.Fatalf("masked company NPWP must be rejected, got %v", err)
	}
	if _, err := handler.CreateCompany(ctx, &payrollv1.CreateCompanyRequest{Code: "MSK", Name: "Mask", Npwp: "***"}); err == nil || code(t, err) != apperr.CodeValidation {
		t.Fatalf("masked company NPWP on create must be rejected, got %v", err)
	}
	in = input("Cuti Big")
	in.CutiTahunan = "9999.95"
	_, err = handler.CreateEmployee(ctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "E701", Input: in})
	if err == nil || code(t, err) != apperr.CodeValidation {
		t.Fatalf("cuti_tahunan above 9999.9 must be rejected, got %v", err)
	}
}

func TestListCursorAndQWildcards(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	mustCreate(t, "E800", "Pct 100% Wide")
	mustCreate(t, "E801", "Plain Name")
	mustCreate(t, "E802", "Under_score")
	for q, want := range map[string]int{"%": 1, "_": 1, "100%": 1} {
		got, err := handler.ListEmployees(ctx, &payrollv1.ListEmployeesRequest{Q: q, Limit: 50})
		if err != nil || len(got.GetData()) != want {
			t.Fatalf("q=%q want %d rows, got %d err=%v", q, want, len(got.GetData()), err)
		}
	}
	first, err := handler.ListEmployees(ctx, &payrollv1.ListEmployeesRequest{Limit: 2})
	if err != nil || len(first.GetData()) != 2 || first.GetNextCursor() == "" {
		t.Fatalf("first page: %v %+v", err, first)
	}
	second, err := handler.ListEmployees(ctx, &payrollv1.ListEmployeesRequest{Limit: 2, Cursor: first.GetNextCursor()})
	if err != nil || len(second.GetData()) == 0 || second.GetData()[0].GetId() <= first.GetData()[1].GetId() {
		t.Fatalf("second page must continue after the cursor: %v %+v", err, second)
	}
	if _, err := handler.ListEmployees(ctx, &payrollv1.ListEmployeesRequest{Cursor: "abc"}); err == nil || code(t, err) != apperr.CodeValidation {
		t.Fatalf("bad cursor must be VALIDATION, got %v", err)
	}
}

func TestEmptyScopeCannotCreateGlobalMasters(t *testing.T) {
	ctx := ctxFor(t, "user-noscope", permsRW...)
	check := func(name string, err error) {
		t.Helper()
		if err == nil || code(t, err) != apperr.CodeDivisionForbidden {
			t.Fatalf("%s: want DIVISION_FORBIDDEN, got %v", name, err)
		}
	}
	_, err := handler.CreateCompany(ctx, &payrollv1.CreateCompanyRequest{Code: "NSC", Name: "No Scope"})
	check("create company", err)
	_, err = handler.CreateCurrency(ctx, &payrollv1.CreateCurrencyRequest{Code: "GBP", MinorUnit: 2})
	check("create currency", err)
	var n int
	if err := ownDB.QueryRow(`SELECT count(*) FROM payroll.pyr_company WHERE pco_code='NSC'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("company must not exist: n=%d err=%v", n, err)
	}
	list, err := handler.ListCompanies(ctx, &payrollv1.ListCompaniesRequest{})
	if err != nil || len(list.GetData()) != 0 {
		t.Fatalf("empty scope must list nothing: %v %+v", err, list)
	}
}

// D-18: company/division masters are guarded per company, not per division, so a
// SOLO-only user may manage divisions of company 1 (including JAKARTA).
func TestDivisionManagementIsCompanyLevel(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...) // SOLO scope only
	if _, err := handler.CreateDivision(ctx, &payrollv1.CreateDivisionRequest{CompanyId: 1, Code: "SURABAYA", Name: "Surabaya"}); err != nil {
		t.Fatalf("create division in own company: %v", err)
	}
	jkt, err := handler.ListDivisions(ctx, &payrollv1.ListDivisionsRequest{CompanyId: 1})
	if err != nil || len(jkt.GetData()) < 3 {
		t.Fatalf("list divisions: %v %+v", err, jkt)
	}
	for _, d := range jkt.GetData() {
		if d.GetCode() == "JAKARTA" {
			if _, err := handler.UpdateDivision(ctx, &payrollv1.UpdateDivisionRequest{CompanyId: 1, Code: "JAKARTA", Name: "Jakarta Raya", IfMatch: d.GetEtag()}); err != nil {
				t.Fatalf("update JAKARTA by SOLO user (company-level, D-18): %v", err)
			}
		}
	}
}

func TestListCompaniesDecryptsOnlyScope(t *testing.T) {
	var other int16
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('OT2','Other2') RETURNING pco_company_id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	list, err := handler.ListCompanies(ctxFor(t, userRW, permsRW...), &payrollv1.ListCompaniesRequest{})
	if err != nil || len(list.GetData()) == 0 {
		t.Fatalf("list: %v", err)
	}
	for _, c := range list.GetData() {
		if int16(c.GetCompanyId()) == other {
			t.Fatal("company outside scope returned")
		}
	}
}
