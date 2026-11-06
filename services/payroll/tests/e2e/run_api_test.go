package e2e

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/period"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T022b: run & period RPCs (07§4–5, D-51/D-53) against PG18 as payroll_app. The engine is a stub (one T1 line
// per employee): this test covers permission, scope, masking, status mapping and the commit → approve → close flow.
const runYYMM = 203501

type stubRules struct{}

func (stubRules) ID() int                                 { return 1 }
func (stubRules) LegacyCompat() bool                      { return true }
func (stubRules) Constant(string) (decimal.Decimal, bool) { return decimal.Zero, false }

var (
	runOnce sync.Once
	runH    *grpcdelivery.RunHandler
)

func runHandler(t *testing.T) *grpcdelivery.RunHandler {
	t.Helper()
	runOnce.Do(func() {
		stub := func(in engine.Input) (engine.Result, error) {
			a := decimal.RequireFromString("1000000")
			return engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a, Tax: true}},
				Header: &engine.Header{TotTunjangan: a}}, nil
		}
		rules := func(context.Context, *sql.Tx, run.Query, int) (run.Rules, error) {
			return run.Rules{Set: stubRules{}}, nil
		}
		w := auditinfra.NewWriter()
		runH = grpcdelivery.NewRunHandler(run.NewServiceWith(appDB, rules, w, run.Runner{Calc: stub}), period.NewService(appDB, w), postgres.NewRunQuery(appDB))
		// Other e2e files leave employees with test-only component codes (T9xx) in the same divisions; the loader
		// accepts only T/P + number, so take them out of payroll. This file sorts last, nothing after it needs them.
		if _, err := ownDB.Exec(`UPDATE payroll.pyr_employee SET pe_sts_payroll = 1 WHERE pe_nik NOT LIKE 'ZRUN%'`); err != nil {
			t.Fatalf("isolate employees: %v", err)
		}
		for _, div := range []string{"SOLO", "JAKARTA"} {
			for _, q := range []string{
				`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_status) VALUES (1,'v1',200001,'ACTIVE') ON CONFLICT DO NOTHING`,
				`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_status, pc_valid_from)
				 VALUES (1,'T001','Tunjangan','Gaji Pokok','ACTIVE',202001) ON CONFLICT (pc_company_id, pc_code) DO NOTHING`,
				`INSERT INTO payroll.pyr_employee (pe_company_id, pe_nik, pe_division, pe_nama, pe_level, pe_tax_id, pe_sts_payroll, pe_orion_sub_acnt, pe_dept)
				 SELECT 1, 'ZRUN' || left('` + div + `',1) || lpad(g::text, 4, '0'), '` + div + `', 'Run ' || g, 'Staff', 'TK', 2, 'ZR` + div + `' || g, 'PROD' FROM generate_series(1, 3) g`,
				`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203501,'` + div + `',26)`,
				`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked)
				 VALUES (1,203501,'` + div + `','{1}','runapi-` + div + `',true)`,
				`INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
				 SELECT s.pas_snapshot_id, 'REKAP', 'ZRUN' || left('` + div + `',1) || lpad(g::text, 4, '0'), '{"MPAR_DAY_WORK":26,"MPAR_PAY_TOT":26}'::jsonb
				 FROM payroll.pyr_att_snapshot s, generate_series(1, 3) g WHERE s.pas_checksum = 'runapi-` + div + `'`,
			} {
				if _, err := ownDB.Exec(q); err != nil {
					t.Fatalf("seed: %v\n%s", err, q)
				}
			}
		}
	})
	return runH
}

var (
	permsRun = []string{"payroll.run.dryrun.execute", "payroll.run.salary.commit", "payroll.period.salary.approve",
		"payroll.period.salary.close", "payroll.period.salary.reopen", "payroll.report.data.view", "payroll.salary.amount.view"}
	permsRunNoAmount = []string{"payroll.run.dryrun.execute", "payroll.run.salary.commit", "payroll.report.data.view"}
)

func createRun(div string, mode payrollv1.RunMode) *payrollv1.CreateRunRequest {
	return &payrollv1.CreateRunRequest{Yymm: runYYMM, Division: div, CompanyId: 1, ProcessDate: "2035-01-25", Mode: mode}
}

func wantCode(t *testing.T, name string, err error, want apperr.Code) {
	t.Helper()
	if got := code(t, err); got != want {
		t.Fatalf("%s: code = %s, want %s", name, got, want)
	}
}

// A SOLO-only user asking for JAKARTA is refused with DIVISION_FORBIDDEN before any query, in all 8 RPCs.
// The by-id reads (GetRun, ListRunLines) cannot see the division of a foreign run (RLS), so they answer
// NOT_FOUND for a run outside the scope, which is the same 403-equivalent denial without leaking existence (D-53).
func TestRunAPI_ScopeDenied(t *testing.T) {
	h := runHandler(t)
	jkt := ctxFor(t, userJkt, permsRun...)
	solo := ctxFor(t, userRW, permsRun...)

	cr, err := h.CreateRun(jkt, createRun("JAKARTA", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Fatalf("jakarta commit: %v", err)
	}
	id := cr.GetData().GetRunId()

	_, err = h.CreateRun(solo, createRun("JAKARTA", payrollv1.RunMode_RUN_MODE_DRY))
	wantCode(t, "CreateRun dry", err, apperr.CodeDivisionForbidden)
	_, err = h.CreateRun(solo, createRun("JAKARTA", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "CreateRun commit", err, apperr.CodeDivisionForbidden)
	_, err = h.ListRuns(solo, &payrollv1.ListRunsRequest{Division: "JAKARTA"})
	wantCode(t, "ListRuns", err, apperr.CodeDivisionForbidden)
	_, err = h.ListPeriods(solo, &payrollv1.ListPeriodsRequest{Division: "JAKARTA"})
	wantCode(t, "ListPeriods", err, apperr.CodeDivisionForbidden)
	_, err = h.ApprovePeriod(solo, &payrollv1.ApprovePeriodRequest{Yymm: runYYMM, Division: "JAKARTA"})
	wantCode(t, "ApprovePeriod", err, apperr.CodeDivisionForbidden)
	_, err = h.ClosePeriod(solo, &payrollv1.ClosePeriodRequest{Yymm: runYYMM, Division: "JAKARTA"})
	wantCode(t, "ClosePeriod", err, apperr.CodeDivisionForbidden)
	_, err = h.ReopenPeriod(solo, &payrollv1.ReopenPeriodRequest{Yymm: runYYMM, Division: "JAKARTA", Reason: "alasan cukup panjang"})
	wantCode(t, "ReopenPeriod", err, apperr.CodeDivisionForbidden)
	_, err = h.GetRun(solo, &payrollv1.GetRunRequest{RunId: id})
	wantCode(t, "GetRun", err, apperr.CodeNotFound)
	_, err = h.ListRunLines(solo, &payrollv1.ListRunLinesRequest{RunId: id})
	wantCode(t, "ListRunLines", err, apperr.CodeNotFound)

	// RLS: the SOLO user sees none of the JAKARTA runs/periods even with an empty filter.
	lr, err := h.ListRuns(solo, &payrollv1.ListRunsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range lr.GetData() {
		if r.GetDivision() != "SOLO" {
			t.Fatalf("ListRuns leaked %s", r.GetDivision())
		}
	}
}

func TestRunAPI_DryCommitApproveClose(t *testing.T) {
	h := runHandler(t)
	ctx := ctxFor(t, userRW, permsRun...)

	start := time.Now()
	dry, err := h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_DRY))
	if err != nil {
		t.Fatalf("dry: %v", err)
	}
	if time.Since(start) > 2*time.Minute {
		t.Fatalf("dry took %s, want <= 2 min", time.Since(start))
	}
	if d := dry.GetData(); d.GetStatus() != "DRY" || d.GetEmployeeCount() != 3 || d.GetTotalTunjangan() != "3000000.00" || d.GetRunId() != 0 {
		t.Fatalf("dry data = %v", d)
	}
	if len(dry.GetEmployees()) != 3 || dry.GetEmployees()[0].GetTakeHome() != "1000000.00" {
		t.Fatalf("dry employees = %v", dry.GetEmployees())
	}
	if n := scalar(t, `SELECT count(*) FROM payroll.pyr_run WHERE pr_division='SOLO' AND pr_yymm=203501`); n != "0" {
		t.Fatalf("dry persisted %s runs", n)
	}

	cm, err := h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	id := cm.GetData().GetRunId()
	if id == 0 || cm.GetData().GetStatus() != "ACTIVE" || cm.GetData().GetTotalTunjangan() != "3000000.00" || cm.GetData().GetProcessDate() != "2035-01-25" {
		t.Fatalf("commit data = %v", cm.GetData())
	}
	got, err := h.GetRun(ctx, &payrollv1.GetRunRequest{RunId: id})
	if err != nil || got.GetData().GetRuleSetId() != 1 {
		t.Fatalf("GetRun: %v %v", got, err)
	}
	lines, err := h.ListRunLines(ctx, &payrollv1.ListRunLinesRequest{RunId: id, PageSize: 2})
	if err != nil || len(lines.GetData()) != 2 || lines.GetPagination().GetTotalItems() != 3 || lines.GetPagination().GetTotalPages() != 2 {
		t.Fatalf("ListRunLines: %v %v", lines, err)
	}
	if l := lines.GetData()[0]; l.GetCompType() != "T" || l.GetAmount() != "1000000.00" {
		t.Fatalf("line = %v", l)
	}

	// A period that is not approved cannot be closed (409 CONFLICT); approve → commit again is PERIOD_APPROVED.
	if _, err = h.ClosePeriod(ctx, &payrollv1.ClosePeriodRequest{Yymm: runYYMM, Division: "SOLO"}); code(t, err) != apperr.CodeConflict {
		t.Fatalf("close before approve: %v", err)
	}
	ap, err := h.ApprovePeriod(ctx, &payrollv1.ApprovePeriodRequest{Yymm: runYYMM, Division: "SOLO"})
	if err != nil || ap.GetData().GetStatus() != "APPROVED" || ap.GetData().GetActiveRunId() != id {
		t.Fatalf("approve: %v %v", ap, err)
	}
	_, err = h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "commit on APPROVED", err, apperr.CodePeriodApproved)
	cl, err := h.ClosePeriod(ctx, &payrollv1.ClosePeriodRequest{Yymm: runYYMM, Division: "SOLO"})
	if err != nil || cl.GetData().GetStatus() != "CLOSED" {
		t.Fatalf("close: %v %v", cl, err)
	}
	_, err = h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "commit on CLOSED", err, apperr.CodePeriodClosed)
	_, err = h.ReopenPeriod(ctx, &payrollv1.ReopenPeriodRequest{Yymm: runYYMM, Division: "SOLO", Reason: "pendek"})
	wantCode(t, "reopen short reason", err, apperr.CodeValidation)
	rp, err := h.ReopenPeriod(ctx, &payrollv1.ReopenPeriodRequest{Yymm: runYYMM, Division: "SOLO", Reason: "koreksi data absensi"})
	if err != nil || rp.GetData().GetStatus() != "DRAFT" || rp.GetData().GetReopenReason() != "koreksi data absensi" {
		t.Fatalf("reopen: %v %v", rp, err)
	}
	pl, err := h.ListPeriods(ctx, &payrollv1.ListPeriodsRequest{Division: "SOLO"})
	if err != nil || len(pl.GetData()) == 0 {
		t.Fatalf("ListPeriods: %v %v", pl, err)
	}
}

// Without payroll.salary.amount.view every nominal is "***"; codes, qty, status and warnings stay (I23).
func TestRunAPI_Masking(t *testing.T) {
	h := runHandler(t)
	ctx := ctxFor(t, userRW, permsRunNoAmount...)
	dry, err := h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_DRY))
	if err != nil {
		t.Fatal(err)
	}
	if d := dry.GetData(); d.GetTotalTunjangan() != "***" || d.GetTotalPotongan() != "***" || d.GetEmployeeCount() != 3 {
		t.Fatalf("masked dry = %v", d)
	}
	if e := dry.GetEmployees()[0]; e.GetTakeHome() != "***" || e.GetTotalTunjangan() != "***" || e.GetNik() == "" {
		t.Fatalf("masked employee = %v", e)
	}
	lr, err := h.ListRuns(ctx, &payrollv1.ListRunsRequest{Division: "SOLO"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range lr.GetData() {
		if r.GetTotalTunjangan() != "***" {
			t.Fatalf("ListRuns not masked: %v", r)
		}
		ln, err := h.ListRunLines(ctx, &payrollv1.ListRunLinesRequest{RunId: r.GetRunId()})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range ln.GetData() {
			if l.GetAmount() != "***" || l.GetBase() != "***" || l.GetRate() != "***" || l.GetCompType() == "" || l.GetCompId() == 0 {
				t.Fatalf("line not masked: %v", l)
			}
		}
	}
}

// Commit needs payroll.run.salary.commit on top of the dry-run permission the interceptor checks.
func TestRunAPI_CommitPermission(t *testing.T) {
	h := runHandler(t)
	ctx := ctxFor(t, userRW, "payroll.run.dryrun.execute")
	_, err := h.CreateRun(ctx, createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	wantCode(t, "commit without permission", err, apperr.CodePermissionDenied)
}

func scalar(t *testing.T, q string) string {
	t.Helper()
	var s string
	if err := ownDB.QueryRow(q).Scan(&s); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return s
}

func TestRunAPI_Validation(t *testing.T) {
	h := runHandler(t)
	ctx := ctxFor(t, userRW, permsRun...)
	bad := createRun("SOLO", payrollv1.RunMode_RUN_MODE_DRY)
	bad.Yymm = 203513 // month 13 passes the proto range but is not a period
	_, err := h.CreateRun(ctx, bad)
	wantCode(t, "bad yymm", err, apperr.CodeValidation)
	withNIK := createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT)
	withNIK.Nik = []string{"ZRUNS0001"}
	_, err = h.CreateRun(ctx, withNIK)
	wantCode(t, "commit with nik", err, apperr.CodeValidation)
	noDate := createRun("SOLO", payrollv1.RunMode_RUN_MODE_DRY)
	noDate.ProcessDate = ""
	if _, err = h.CreateRun(ctx, noDate); err != nil {
		t.Fatalf("dry without process_date: %v", err)
	}
	noDate.Mode = payrollv1.RunMode_RUN_MODE_COMMIT
	_, err = h.CreateRun(ctx, noDate)
	wantCode(t, "commit without process_date", err, apperr.CodeValidation)
}

// ResolveRuleSet: reprocess keeps the rule set of the ACTIVE run; no ACTIVE rule set = RULE_INCOMPLETE (422).
func TestRunAPI_ResolveRuleSet(t *testing.T) {
	h := runHandler(t)
	ctx := ctxFor(t, userRW, permsRun...)
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_status) VALUES (2,'v2',203601,'ACTIVE') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	// 203502 has no run: latest ACTIVE rule set with effective_from <= yymm is v1 (v2 starts 203601).
	req := createRun("SOLO", payrollv1.RunMode_RUN_MODE_DRY)
	req.Yymm = 203502
	if _, err := ownDB.Exec(`INSERT INTO payroll.pyr_attd_days (pad_company_id, pad_yymm, pad_division, pad_days) VALUES (1,203502,'SOLO',26) ON CONFLICT DO NOTHING;
		INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum, pas_locked) VALUES (1,203502,'SOLO','{1}','runapi-0502',true);
		INSERT INTO payroll.pyr_att_snapshot_row (pasr_snapshot_id, pasr_kind, pasr_nik, pasr_data)
		SELECT s.pas_snapshot_id, 'REKAP', 'ZRUNS000' || g, '{"MPAR_DAY_WORK":26}'::jsonb FROM payroll.pyr_att_snapshot s, generate_series(1,3) g WHERE s.pas_checksum='runapi-0502'`); err != nil {
		t.Fatal(err)
	}
	got, err := h.CreateRun(ctx, req)
	if err != nil || got.GetData().GetRuleSetId() != 1 {
		t.Fatalf("effective rule set: %v %v", got, err)
	}
	// Reprocess: the ACTIVE run of 203502 uses rule set 1; a later ACTIVE rule set 2 (203601) must not take over
	// even when its effective_from would match, so point the run at 2 and expect 2 to be kept.
	req.Mode = payrollv1.RunMode_RUN_MODE_COMMIT
	cm, err := h.CreateRun(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_rule_set SET prs_effective_from = 200001 WHERE prs_id = 2`); err != nil {
		t.Fatal(err)
	}
	again, err := h.CreateRun(ctx, req)
	if err != nil || again.GetData().GetRuleSetId() != cm.GetData().GetRuleSetId() {
		t.Fatalf("reprocess rule set = %v, want %d (err %v)", again.GetData().GetRuleSetId(), cm.GetData().GetRuleSetId(), err)
	}
	// No ACTIVE rule set at all for an early period.
	req.Yymm, req.Mode = 200002, payrollv1.RunMode_RUN_MODE_DRY
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_rule_set SET prs_effective_from = 200101`); err != nil {
		t.Fatal(err)
	}
	_, err = h.CreateRun(ctx, req)
	wantCode(t, "no rule set", err, apperr.CodeRuleIncomplete)
	if _, err := ownDB.Exec(`UPDATE payroll.pyr_rule_set SET prs_effective_from = 200001 WHERE prs_id = 1; UPDATE payroll.pyr_rule_set SET prs_effective_from = 203601 WHERE prs_id = 2`); err != nil {
		t.Fatal(err)
	}
}

// By-id reads of another division are NOT_FOUND; masking applies to GetRun and ListRunLines; an ambiguous
// division (same code in two companies of the caller scope) is refused for the period RPCs.
func TestRunAPI_ByIDScopeMaskAmbiguity(t *testing.T) {
	h := runHandler(t)
	full := ctxFor(t, userRW, permsRun...)
	cm, err := h.CreateRun(ctxFor(t, userJkt, permsRun...), createRun("JAKARTA", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Skipf("jakarta run exists already: %v", err) // period may be APPROVED by TestRunAPI_ScopeDenied order
	}
	id := cm.GetData().GetRunId()
	_, err = h.GetRun(full, &payrollv1.GetRunRequest{RunId: id})
	wantCode(t, "GetRun other division", err, apperr.CodeNotFound)

	solo, err := h.CreateRun(full, createRun("SOLO", payrollv1.RunMode_RUN_MODE_COMMIT))
	if err != nil {
		t.Skipf("solo period not open: %v", err)
	}
	masked := ctxFor(t, userRW, permsRunNoAmount...)
	g, err := h.GetRun(masked, &payrollv1.GetRunRequest{RunId: solo.GetData().GetRunId()})
	if err != nil || g.GetData().GetTotalTunjangan() != "***" {
		t.Fatalf("GetRun masked: %v %v", g, err)
	}
	ln, err := h.ListRunLines(masked, &payrollv1.ListRunLinesRequest{RunId: solo.GetData().GetRunId()})
	if err != nil || len(ln.GetData()) == 0 || ln.GetData()[0].GetAmount() != "***" {
		t.Fatalf("ListRunLines masked: %v %v", ln, err)
	}

	// Ambiguity: company 2 also has a SOLO division and period for the same user.
	var co int
	if err := ownDB.QueryRow(`INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('AMB','Ambig') RETURNING pco_company_id`).Scan(&co); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO payroll.pyr_division_ref VALUES (` + itoa(co) + `,'SOLO','Solo')`,
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_company_id, pus_division) VALUES ('` + userRW + `',` + itoa(co) + `,'SOLO')`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to) VALUES (` + itoa(co) + `,203501,'SOLO','2034-12-26','2035-01-25')`,
	} {
		if _, err := ownDB.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	_, err = h.ApprovePeriod(ctxFor(t, userRW, permsRun...), &payrollv1.ApprovePeriodRequest{Yymm: runYYMM, Division: "SOLO"})
	wantCode(t, "ambiguous period", err, apperr.CodeValidation)
}

func itoa(i int) string { return strconv.Itoa(i) }
