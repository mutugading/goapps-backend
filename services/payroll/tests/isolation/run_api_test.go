package isolation

// I22 and I23 (09§6.4-6.5) through the real RunHandler (in-process gRPC handler, identity and permissions placed
// in the context exactly like the auth interceptor does), against PG18 as payroll_app.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/period"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

var (
	permsRunAll = []string{"payroll.run.dryrun.execute", "payroll.run.salary.commit", "payroll.period.salary.approve",
		"payroll.period.salary.close", "payroll.period.salary.reopen", "payroll.report.data.view", "payroll.salary.amount.view"}
	permsRunNoAmount = []string{"payroll.run.dryrun.execute", "payroll.run.salary.commit", "payroll.report.data.view"}
)

func newRunHandler() *grpcdelivery.RunHandler {
	w := audit.NewWriter()
	rules := func(context.Context, *sql.Tx, run.Query, int) (run.Rules, error) {
		return run.Rules{Set: i25Rules{}}, nil
	}
	return grpcdelivery.NewRunHandler(run.NewServiceWith(appDB, rules, w, run.Runner{Calc: i25Calc}), period.NewService(appDB, w), postgres.NewRunQuery(appDB))
}

func ctxPerms(t *testing.T, userID string, perms ...string) context.Context {
	t.Helper()
	return context.WithValue(ctxFor(t, userID), grpcdelivery.AuthPermissionsKey, perms)
}

func runReq(div string, mode payrollv1.RunMode) *payrollv1.CreateRunRequest {
	return &payrollv1.CreateRunRequest{Yymm: i25Period, Division: div, CompanyId: 1, ProcessDate: "2026-11-25", Mode: mode}
}

func wantCode(t *testing.T, what string, err error, want apperr.Code) {
	t.Helper()
	got, ok := grpcdelivery.CodeFromError(err)
	if !ok || got != want {
		t.Errorf("%s: code = %q (err %v), want %s", what, got, err, want)
	}
}

// I22: a JWT user scoped to SOLO is refused (403 DIVISION_FORBIDDEN) on every run/period RPC for JAKARTA
// and sees no JAKARTA rows when it lists without a division filter.
func TestI22_ScopedUserDeniedOtherDivision(t *testing.T) {
	h := newRunHandler()
	solo := ctxPerms(t, userSolo, permsRunAll...)
	count := func(q string) int {
		t.Helper()
		var n int
		if err := ownDB.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	const runsQ = `SELECT count(*) FROM payroll.pyr_run WHERE pr_division='JAKARTA' AND pr_yymm=202611`
	const perQ = `SELECT count(*) FROM payroll.pyr_period WHERE pp_division='JAKARTA' AND pp_yymm IN (202611, 202609)`
	const audQ = `SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_actor='` + userSolo + `'`
	runs0, per0, aud0 := count(runsQ), count(perQ), count(audQ)

	_, err := h.CreateRun(solo, runReq("JAKARTA", payrollv1.RunMode_RUN_MODE_DRY))
	wantCode(t, "CreateRun dry", err, apperr.CodeDivisionForbidden)
	_, err = h.CreateRun(solo, runReq("JAKARTA", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "CreateRun commit", err, apperr.CodeDivisionForbidden)
	_, err = h.ListRuns(solo, &payrollv1.ListRunsRequest{Division: "JAKARTA"})
	wantCode(t, "ListRuns", err, apperr.CodeDivisionForbidden)
	_, err = h.ListPeriods(solo, &payrollv1.ListPeriodsRequest{Division: "JAKARTA"})
	wantCode(t, "ListPeriods", err, apperr.CodeDivisionForbidden)
	_, err = h.ApprovePeriod(solo, &payrollv1.ApprovePeriodRequest{Yymm: 202609, Division: "JAKARTA"})
	wantCode(t, "ApprovePeriod", err, apperr.CodeDivisionForbidden)
	_, err = h.ClosePeriod(solo, &payrollv1.ClosePeriodRequest{Yymm: 202609, Division: "JAKARTA"})
	wantCode(t, "ClosePeriod", err, apperr.CodeDivisionForbidden)
	_, err = h.ReopenPeriod(solo, &payrollv1.ReopenPeriodRequest{Yymm: 202609, Division: "JAKARTA", Reason: "alasan cukup panjang"})
	wantCode(t, "ReopenPeriod", err, apperr.CodeDivisionForbidden)

	// The seed holds a JAKARTA run (202609, ACTIVE); by-id reads answer NOT_FOUND for it (RLS, D-53), 0 rows.
	var jktRun int64
	if err := ownDB.QueryRow(`SELECT pr_id FROM payroll.pyr_run WHERE pr_division='JAKARTA' LIMIT 1`).Scan(&jktRun); err != nil {
		t.Fatalf("seed JAKARTA run: %v", err)
	}
	_, err = h.GetRun(solo, &payrollv1.GetRunRequest{RunId: jktRun})
	wantCode(t, "GetRun", err, apperr.CodeNotFound)
	_, err = h.ListRunLines(solo, &payrollv1.ListRunLinesRequest{RunId: jktRun})
	wantCode(t, "ListRunLines", err, apperr.CodeNotFound)

	if r, p, a := count(runsQ), count(perQ), count(audQ); r != runs0 || p != per0 || a != aud0 {
		t.Errorf("denied calls left side effects: runs %d->%d, periods %d->%d, audit %d->%d", runs0, r, per0, p, aud0, a)
	}

	lr, err := h.ListRuns(solo, &payrollv1.ListRunsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lr.GetData()) == 0 {
		t.Fatal("SOLO user sees no SOLO run; fixture broken")
	}
	for _, r := range lr.GetData() {
		if r.GetDivision() != "SOLO" {
			t.Errorf("ListRuns leaked division %s", r.GetDivision())
		}
	}
	pr, err := h.ListPeriods(solo, &payrollv1.ListPeriodsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.GetData()) == 0 {
		t.Fatal("SOLO user sees no period; leak check would be vacuous")
	}
	for _, p := range pr.GetData() {
		if p.GetDivision() != "SOLO" {
			t.Errorf("ListPeriods leaked division %s", p.GetDivision())
		}
	}
}

// I23: without payroll.salary.amount.view every nominal (and the warning text) is "***"; with it, numbers.
func TestI23_NominalMaskedWithoutPermission(t *testing.T) {
	// T001 is normally loaded by the master ETL (I25 runs after this file); make sure it exists for the seed.
	var hadComp int
	if err := ownDB.QueryRow(`SELECT count(*) FROM payroll.pyr_component WHERE pc_company_id=1 AND pc_code='T001'`).Scan(&hadComp); err != nil {
		t.Fatal(err)
	}
	if hadComp == 0 {
		t.Cleanup(func() {
			if _, err := ownDB.Exec(`DELETE FROM payroll.pyr_component WHERE pc_company_id=1 AND pc_code='T001'`); err != nil {
				t.Errorf("cleanup component: %v", err)
			}
		})
	}
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
		VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`); err != nil {
		t.Fatalf("component: %v", err)
	}
	i25Seed(t)
	h := newRunHandler()
	full := ctxPerms(t, userSolo, permsRunAll...)
	blind := ctxPerms(t, userSolo, permsRunNoAmount...)

	cm, err := h.CreateRun(full, runReq("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	id := cm.GetData().GetRunId()
	got, err := h.GetRun(blind, &payrollv1.GetRunRequest{RunId: id})
	if err != nil {
		t.Fatal(err)
	}
	if d := got.GetData(); d.GetTotalTunjangan() != "***" || d.GetTotalPotongan() != "***" || d.GetDivision() != "SOLO" {
		t.Errorf("GetRun without permission = %v", d)
	}
	lines, err := h.ListRunLines(blind, &payrollv1.ListRunLinesRequest{RunId: id})
	if err != nil || len(lines.GetData()) == 0 {
		t.Fatalf("ListRunLines: %v %v", lines, err)
	}
	for _, l := range lines.GetData() {
		if l.GetAmount() != "***" || l.GetBase() != "***" || l.GetRate() != "***" {
			t.Errorf("line not masked: %v", l)
		}
	}
	lr, err := h.ListRuns(blind, &payrollv1.ListRunsRequest{Division: "SOLO"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range lr.GetData() {
		found = found || r.GetRunId() == id
		if r.GetTotalTunjangan() != "***" || r.GetTotalPotongan() != "***" {
			t.Errorf("ListRuns not masked: %v", r)
		}
	}
	if !found {
		t.Errorf("ListRuns without permission does not list run %d", id)
	}
	dry, err := h.CreateRun(blind, runReq("SOLO", payrollv1.RunMode_RUN_MODE_DRY))
	if err != nil {
		t.Fatalf("dry: %v", err)
	}
	if dry.GetData().GetTotalTunjangan() != "***" {
		t.Errorf("dry total = %q", dry.GetData().GetTotalTunjangan())
	}
	if len(dry.GetEmployees()) == 0 {
		t.Error("dry run returned no employees; mask check would be vacuous")
	}
	for _, e := range dry.GetEmployees() {
		if e.GetTotalTunjangan() != "***" || e.GetTotalPotongan() != "***" || e.GetTakeHome() != "***" {
			t.Errorf("dry employee not masked: %v", e)
		}
	}

	seen, err := h.ListRunLines(full, &payrollv1.ListRunLinesRequest{RunId: id})
	if err != nil || len(seen.GetData()) == 0 {
		t.Fatalf("ListRunLines with permission: %v %v", seen, err)
	}
	if a := seen.GetData()[0].GetAmount(); !strings.HasPrefix(a, "8765432") {
		t.Errorf("with permission amount = %q", a)
	}
}
