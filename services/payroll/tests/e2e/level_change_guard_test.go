package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

func seedPeriod(t *testing.T, yymm int, division, status string) {
	t.Helper()
	if _, err := ownDB.ExecContext(context.Background(), `INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
	  VALUES (1,$1,$2,'2020-01-26','2020-02-25',$3::payroll.period_status) ON CONFLICT (pp_company_id, pp_yymm, pp_division) DO UPDATE SET pp_status=EXCLUDED.pp_status`,
		yymm, division, status); err != nil {
		t.Fatalf("seed period: %v", err)
	}
}

func seedRule(t *testing.T, level, grp string, rate string, sts int, tunj int) {
	t.Helper()
	if _, err := ownDB.ExecContext(context.Background(), `INSERT INTO payroll.pyr_level_rule VALUES ($1,$2,$3,$4,1,$4,$5,'T1_7_55') ON CONFLICT DO NOTHING`,
		level, grp, rate, sts, tunj); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
}

func createAt(t *testing.T, nik, level, join string) *payrollv1.Employee {
	t.Helper()
	in := input("Guard " + nik)
	in.Level, in.DateJoin = level, join
	resp, err := handler.CreateEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: nik, Input: in})
	if err != nil {
		t.Fatalf("create %s: %v", nik, err)
	}
	return resp.GetData()
}

func componentsOf(t *testing.T, nik string) map[string]*payrollv1.EmployeeComponent {
	t.Helper()
	got, err := handler.GetEmployeeComponents(ctxFor(t, userRW, permsRW...), &payrollv1.GetEmployeeComponentsRequest{Nik: nik, CompanyId: 1, Division: "SOLO"})
	if err != nil {
		t.Fatalf("get components %s: %v", nik, err)
	}
	return codesOf(got.GetItems())
}

// D-21: a level whose rule needs components the company lacks must not break CreateEmployee.
func TestCreateEmployee_RuleComponentsMissingIsSkipped(t *testing.T) {
	bg := context.Background()
	var co int
	if err := ownDB.QueryRowContext(bg, `INSERT INTO payroll.pyr_company (pco_code, pco_name) VALUES ('GDX','Guard X') RETURNING pco_company_id`).Scan(&co); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO payroll.pyr_division_ref VALUES ($1,'SOLO','Solo')`,
		`INSERT INTO payroll.pyr_user_scope (pus_user_id, pus_company_id, pus_division) VALUES ('` + userRW + `',$1,'SOLO')`,
	} {
		if _, err := ownDB.ExecContext(bg, q, co); err != nil {
			t.Fatalf("seed company scope: %v", err)
		}
	}
	seedRule(t, "LV-NOCOMP", "SHIFT", "0.0877", 2, 2200) // needs T006/T034; the new company has no catalog
	in := input("Tanpa Komponen")
	in.Level = "LV-NOCOMP"
	if _, err := handler.CreateEmployee(ctxFor(t, userRW, permsRW...), &payrollv1.CreateEmployeeRequest{CompanyId: int32(co), Division: "SOLO", Nik: "GD001", Input: in}); err != nil { //nolint:gosec // small test id
		t.Fatalf("create must not fail when rule components are missing: %v", err)
	}
	got, err := handler.GetEmployeeComponents(ctxFor(t, userRW, permsRW...), &payrollv1.GetEmployeeComponentsRequest{Nik: "GD001", CompanyId: int32(co), Division: "SOLO"}) //nolint:gosec // small test id
	if err != nil || len(got.GetItems()) != 0 {
		t.Fatalf("attachments = %v (%v), want none", got.GetItems(), err)
	}
}

// date_join in a CLOSED period must not write valid_from into that period.
func TestCreateEmployee_ClosedJoinPeriodLeavesValidFromUnset(t *testing.T) {
	seedPeriod(t, 202003, "SOLO", "CLOSED")
	tmpl := mustComponent(t, "T9G1", "T", "Guard template")
	putLevelTemplate(t, "LV-GUARD", tmpl.GetSysid())
	createAt(t, "GD002", "LV-GUARD", "2020-03-05")
	got := componentsOf(t, "GD002")
	if got["T9G1"] == nil {
		t.Fatalf("template not attached: %v", got)
	}
	if got["T9G1"].GetValidFrom() != 0 {
		t.Fatalf("valid_from = %d, want unset for a CLOSED join period", got["T9G1"].GetValidFrom())
	}
}

// A later non-OPEN period (here DRAFT) after the effective yymm blocks a level-change; so does CLOSED.
func TestApplyLevelChange_BlockedByLaterNonOpenPeriod(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	seedRule(t, "LV-GA", "STAFF", "0", 1, 1000)
	seedRule(t, "LV-GB", "STAFF", "0", 1, 2000)
	emp := createAt(t, "GD003", "LV-GA", "2026-05-10")
	seedPeriod(t, 202804, "SOLO", "DRAFT")
	defer func() {
		if _, err := ownDB.ExecContext(context.Background(), `DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=202804 AND pp_division='SOLO'`); err != nil {
			t.Errorf("cleanup period: %v", err)
		}
	}()
	_, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD003", CompanyId: 1, Division: "SOLO",
		NewLevel: "LV-GB", EffectiveYymm: 202801, IfMatch: emp.GetEtag()})
	if code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("apply with later DRAFT period = %v, want PERIOD_CLOSED", err)
	}
}

// Same level is a no-op: no version bump, no audit row.
func TestApplyLevelChange_SameLevelIsNoOp(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	seedRule(t, "LV-GC", "STAFF", "0", 1, 1000)
	emp := createAt(t, "GD004", "LV-GC", "2026-05-10")
	ap, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD004", CompanyId: 1, Division: "SOLO",
		NewLevel: "LV-GC", EffectiveYymm: 202701, IfMatch: emp.GetEtag()})
	if err != nil {
		t.Fatalf("same-level apply: %v", err)
	}
	if ap.GetData().GetEtag() != emp.GetEtag() {
		t.Fatalf("etag changed on no-op: %s -> %s", emp.GetEtag(), ap.GetData().GetEtag())
	}
	var n int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_audit_log WHERE pal_action='LEVEL_CHANGE' AND pal_entity_key=$1`, strconv.FormatInt(emp.GetId(), 10)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("audit rows for no-op = %d, want 0", n)
	}
}

// Level-change strict mode (D-21): a rule component absent for the employee's company is a validation error.
func TestPreviewLevelChange_RuleComponentMissingIsValidation(t *testing.T) {
	seedRule(t, "LV-NEEDS-X", "STAFF", "0", 1, 500)
	if _, err := ownDB.ExecContext(context.Background(), `UPDATE payroll.pyr_component SET pc_status='RETIRED' WHERE pc_code='T035' AND pc_company_id=1`); err != nil {
		t.Skipf("cannot retire T035 in this schema: %v", err)
	}
	defer func() {
		if _, err := ownDB.ExecContext(context.Background(), `UPDATE payroll.pyr_component SET pc_status='ACTIVE' WHERE pc_code='T035' AND pc_company_id=1`); err != nil {
			t.Errorf("restore T035: %v", err)
		}
	}()
	mustCreate(t, "GD006", "Strict")
	_, err := handler.PreviewLevelChange(ctxFor(t, userRW, permsRW...), &payrollv1.PreviewLevelChangeRequest{Nik: "GD006", CompanyId: 1, Division: "SOLO", NewLevel: "LV-NEEDS-X", EffectiveYymm: 202701})
	if code(t, err) != apperr.CodeValidation {
		t.Fatalf("preview with missing rule component = %v, want VALIDATION", err)
	}
}

// Level groups GS / DRIVER / DIRUMAHKAN are non-staff: they take T006/T034 and drop T002-T004.
func TestLevelChange_NonStaffGroupsAndPutDropsManual(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	t002 := mustComponentOnce(t, "T002", "Tunj 2")
	mustComponentOnce(t, "T006", "OT in work")
	mustComponentOnce(t, "T034", "Tunj shift")
	manual := mustComponent(t, "T9G9", "T", "Manual guard")
	for lvl, grp := range map[string]string{"LV-GS": "GS", "LV-DRV": "DRIVER", "LV-DRM": "DIRUMAHKAN"} {
		seedRule(t, lvl, grp, "0.05", 2, 1500)
		putLevelTemplate(t, lvl, t002.GetSysid())
	}
	putLevelTemplate(t, "LV-STF-G", t002.GetSysid())
	emp := createAt(t, "GD007", "LV-STF-G", "2026-05-10")
	if _, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "GD007", CompanyId: 1, Division: "SOLO",
		Items: append(inputsOf(componentsOf(t, "GD007")), &payrollv1.EmployeeComponentInput{ComponentSysid: manual.GetSysid(), Amount: "1"})}); err != nil {
		t.Fatalf("put manual: %v", err)
	}
	etag := emp.GetEtag()
	for _, lvl := range []string{"LV-GS", "LV-DRV", "LV-DRM"} {
		ap, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD007", CompanyId: 1, Division: "SOLO", NewLevel: lvl, EffectiveYymm: 202801, IfMatch: etag})
		if err != nil {
			t.Fatalf("apply %s: %v", lvl, err)
		}
		etag = ap.GetData().GetEtag()
		m := componentsOf(t, "GD007")
		if m["T006"].GetAmount() != "0.05" || m["T034"].GetAmount() != "1500" || m["T002"] != nil || m["T9G9"].GetSource() != "MANUAL" {
			t.Fatalf("%s attachments = %v", lvl, m)
		}
	}
	// PUT without the MANUAL component detaches it and snapshots it to hist.
	if _, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "GD007", CompanyId: 1, Division: "SOLO", Items: nil}); err != nil {
		t.Fatalf("put empty: %v", err)
	}
	if got := componentsOf(t, "GD007"); len(got) != 0 {
		t.Fatalf("after empty PUT = %v", got)
	}
	var hist int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_employee_component_hist WHERE pec_nik='GD007' AND pec_component_sysid=$1`, manual.GetSysid()).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("hist rows for detached MANUAL = %d (%v)", hist, err)
	}
	// A CHANGE (T006 amount differs between levels) also leaves hist rows.
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_employee_component_hist WHERE pec_nik='GD007' AND pec_source='TEMPLATE'`).Scan(&hist); err != nil || hist == 0 {
		t.Fatalf("hist rows for TEMPLATE changes/removals = %d (%v)", hist, err)
	}
}

func mustComponentOnce(t *testing.T, code, name string) *payrollv1.Component {
	t.Helper()
	resp, err := handler.ListComponents(ctxFor(t, userRW, permsRW...), &payrollv1.ListComponentsRequest{CompanyId: 1, Q: code, Limit: 50})
	if err == nil {
		for _, c := range resp.GetData() {
			if c.GetCode() == code {
				return c
			}
		}
	}
	return mustComponent(t, code, "T", name)
}

func inputsOf(m map[string]*payrollv1.EmployeeComponent) []*payrollv1.EmployeeComponentInput {
	out := make([]*payrollv1.EmployeeComponentInput, 0, len(m))
	for _, it := range m {
		out = append(out, &payrollv1.EmployeeComponentInput{ComponentSysid: it.GetComponentSysid(), Sts: it.GetSts(), Tax: it.GetTax(), Amount: it.GetAmount(), FrzFlag: it.GetFrzFlag()})
	}
	return out
}

// Migration 000023 down then up with data, inside a rolled-back transaction (DDL is transactional).
func TestMigration000023_DownUpWithData(t *testing.T) {
	bg := context.Background()
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tx, err := ownDB.BeginTx(bg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // test cleanup
	// Down must refuse while rows without legacy (type,id) exist.
	if _, err = tx.ExecContext(bg, `SAVEPOINT s1`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(bg, read("000023_employee_component_sysid.down.sql")); err == nil {
		var n int
		if qerr := tx.QueryRowContext(bg, `SELECT count(*) FROM payroll.pyr_employee_component WHERE pec_type IS NULL`).Scan(&n); qerr != nil || n != 0 {
			t.Fatalf("down succeeded although %d rows lack (type,id) (%v)", n, qerr)
		}
	} else if _, err = tx.ExecContext(bg, `ROLLBACK TO SAVEPOINT s1`); err != nil {
		t.Fatal(err)
	}
	// Clear the data that blocks down, then down -> up must be repeatable (idempotent).
	if _, err = tx.ExecContext(bg, `DELETE FROM payroll.pyr_employee_component_hist; DELETE FROM payroll.pyr_employee_component`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"000023_employee_component_sysid.down.sql", "000023_employee_component_sysid.down.sql", "000023_employee_component_sysid.up.sql"} {
		if _, err = tx.ExecContext(bg, read(f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

func TestLevelChange_InvalidEffectiveYymmRefused(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	seedRule(t, "LV-GV", "STAFF", "0", 1, 1000)
	seedRule(t, "LV-GW", "STAFF", "0", 1, 2000)
	emp := createAt(t, "GD008", "LV-GV", "2026-05-10")
	for _, y := range []int32{0, 999999, 202613} {
		if _, err := handler.PreviewLevelChange(ctx, &payrollv1.PreviewLevelChangeRequest{Nik: "GD008", CompanyId: 1, Division: "SOLO", NewLevel: "LV-GW", EffectiveYymm: y}); code(t, err) != apperr.CodeValidation {
			t.Fatalf("preview yymm=%d = %v, want VALIDATION", y, err)
		}
		if _, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD008", CompanyId: 1, Division: "SOLO", NewLevel: "LV-GW", EffectiveYymm: y, IfMatch: emp.GetEtag()}); code(t, err) != apperr.CodeValidation {
			t.Fatalf("apply yymm=%d = %v, want VALIDATION", y, err)
		}
	}
}

// December -> January: a CLOSED 203112 must not block effective 203201 (only periods >= effective count),
// while effective 203112 is blocked. A non-OPEN period of another division never blocks (JKT vs SOLO).
func TestApplyLevelChange_DecemberJanuaryAndOtherDivision(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	seedRule(t, "LV-GD", "STAFF", "0", 1, 1000)
	seedRule(t, "LV-GE", "STAFF", "0", 1, 2000)
	emp := createAt(t, "GD009", "LV-GD", "2026-05-10")
	seedPeriod(t, 203112, "SOLO", "CLOSED")
	seedPeriod(t, 203301, "JAKARTA", "CLOSED") // other division, later than the effective period
	t.Cleanup(func() {
		if _, err := ownDB.ExecContext(context.Background(), `DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm IN (203112,203301)`); err != nil {
			t.Errorf("cleanup periods: %v", err)
		}
	})
	if _, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD009", CompanyId: 1, Division: "SOLO", NewLevel: "LV-GE", EffectiveYymm: 203112, IfMatch: emp.GetEtag()}); code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("effective in CLOSED Dec = %v, want PERIOD_CLOSED", err)
	}
	if _, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "GD009", CompanyId: 1, Division: "SOLO", NewLevel: "LV-GE", EffectiveYymm: 203201, IfMatch: emp.GetEtag()}); err != nil {
		t.Fatalf("effective Jan after CLOSED Dec (and JKT CLOSED later) = %v, want ok", err)
	}
}
