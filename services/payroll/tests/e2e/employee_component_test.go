package e2e

import (
	"context"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

func codesOf(items []*payrollv1.EmployeeComponent) map[string]*payrollv1.EmployeeComponent {
	out := map[string]*payrollv1.EmployeeComponent{}
	for _, it := range items {
		out[it.GetComponentCode()] = it
	}
	return out
}

func putLevelTemplate(t *testing.T, level string, sysids ...int64) {
	t.Helper()
	items := make([]*payrollv1.TemplateItem, len(sysids))
	for i, id := range sysids {
		items[i] = &payrollv1.TemplateItem{ComponentSysid: id, Sts: "F", Amount: "100"}
	}
	if _, err := handler.PutComponentTemplate(ctxFor(t, userRW, permsRW...), &payrollv1.PutComponentTemplateRequest{Level: level, Items: items}); err != nil {
		t.Fatalf("put template %s: %v", level, err)
	}
}

func TestLevelChangeReplacesTemplateAttachmentsAndKeepsManual(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	bg := context.Background()
	// Components the level rule needs (T006/T034/T035/T036) plus T002 and a manual-only one.
	t002 := mustComponent(t, "T002", "T", "Tunj 2")
	mustComponent(t, "T006", "T", "OT in work")
	mustComponent(t, "T034", "T", "Tunj shift")
	t035 := mustComponent(t, "T035", "T", "Tunj shift staff")
	t036 := mustComponent(t, "T036", "T", "Shift adj")
	t900 := mustComponent(t, "T9B1", "T", "Manual only")

	for _, s := range []string{
		`INSERT INTO payroll.pyr_level_rule VALUES ('LV-SHIFT','SHIFT',0.0877,2,1,2,2200,'T1_7_55')`,
		`INSERT INTO payroll.pyr_level_rule VALUES ('LV-STAFF','STAFF',0,1,1,1,3300,'T1')`,
	} {
		if _, err := ownDB.ExecContext(bg, s); err != nil {
			t.Fatalf("seed level rule: %v", err)
		}
	}
	putLevelTemplate(t, "LV-SHIFT", t002.GetSysid(), t036.GetSysid()) // T36 only gets its frz from the rule (02§5.1)
	putLevelTemplate(t, "LV-STAFF", t002.GetSysid())

	// Create under the staff level: template + rule are attached (replaces TRG_EMP_MAST INSERT).
	in := input("Level Uji")
	in.Level = "LV-STAFF"
	created, err := handler.CreateEmployee(ctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "LC001", Input: in})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := handler.GetEmployeeComponents(ctx, &payrollv1.GetEmployeeComponentsRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO"})
	if err != nil {
		t.Fatal(err)
	}
	have := codesOf(got.GetItems())
	if have["T002"] == nil || have["T035"] == nil || have["T006"] != nil || have["T034"] != nil ||
		have["T035"].GetAmount() != "3300" || have["T002"].GetSource() != "TEMPLATE" {
		t.Fatalf("create attachments = %v", have)
	}

	// Add a MANUAL component.
	items := []*payrollv1.EmployeeComponentInput{}
	for _, it := range got.GetItems() {
		items = append(items, &payrollv1.EmployeeComponentInput{ComponentSysid: it.GetComponentSysid(), Sts: it.GetSts(), Tax: it.GetTax(),
			Amount: it.GetAmount(), FrzFlag: it.GetFrzFlag()})
	}
	items = append(items, &payrollv1.EmployeeComponentInput{ComponentSysid: t900.GetSysid(), Amount: "7.5"})
	put, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO", Items: items})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if m := codesOf(put.GetItems()); m["T9B1"].GetSource() != "MANUAL" || m["T002"].GetSource() != "TEMPLATE" {
		t.Fatalf("put sources = %v", m)
	}

	// Preview to a shift level: T2 (dropped for non-staff) and T35 removed, T6/T34/T36 added, MANUAL T9B1 untouched, nothing written yet.
	pv, err := handler.PreviewLevelChange(ctx, &payrollv1.PreviewLevelChangeRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO", NewLevel: "LV-SHIFT", EffectiveYymm: 202610})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	acts := map[string]string{}
	for _, c := range pv.GetChanges() {
		acts[c.GetComponentCode()] = c.GetAction()
	}
	if acts["T002"] != "REMOVE" || acts["T035"] != "REMOVE" || acts["T006"] != "ADD" || acts["T034"] != "ADD" || acts["T036"] != "ADD" || acts["T9B1"] != "" {
		t.Fatalf("preview actions = %v", acts)
	}
	if again, _ := handler.GetEmployeeComponents(ctx, &payrollv1.GetEmployeeComponentsRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO"}); codesOf(again.GetItems())["T006"] != nil {
		t.Fatal("preview wrote data")
	}

	// Apply: stale If-Match is 412; a CLOSED period is refused; then it applies.
	if _, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO", NewLevel: "LV-SHIFT", EffectiveYymm: 202610, IfMatch: "99"}); code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("stale if-match = %v", err)
	}
	if _, err := ownDB.ExecContext(bg, `INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status) VALUES (1,202601,'SOLO','2025-12-26','2026-01-25','CLOSED')`); err != nil {
		t.Fatalf("seed period: %v", err)
	}
	if _, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO", NewLevel: "LV-SHIFT", EffectiveYymm: 202601, IfMatch: created.GetData().GetEtag()}); code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("closed period = %v", err)
	}
	ap, err := handler.ApplyLevelChange(ctx, &payrollv1.ApplyLevelChangeRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO", NewLevel: "LV-SHIFT", EffectiveYymm: 202610, IfMatch: created.GetData().GetEtag()})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if ap.GetData().GetLevel() != "LV-SHIFT" || ap.GetData().GetEtag() == created.GetData().GetEtag() {
		t.Fatalf("employee after apply = %+v", ap.GetData())
	}
	after, _ := handler.GetEmployeeComponents(ctx, &payrollv1.GetEmployeeComponentsRequest{Nik: "LC001", CompanyId: 1, Division: "SOLO"})
	m := codesOf(after.GetItems())
	if m["T035"] != nil || m["T002"] != nil || m["T006"].GetAmount() != "0.0877" || m["T034"].GetAmount() != "2200" || m["T9B1"].GetSource() != "MANUAL" ||
		m["T9B1"].GetAmount() != "7.5" || m["T036"].GetFrzFlag() != 1 {
		t.Fatalf("after apply = %v", m)
	}
	var hist int
	if err := ownDB.QueryRowContext(bg, `SELECT count(*) FROM payroll.pyr_employee_component_hist WHERE pec_nik='LC001' AND pec_component_sysid=$1`, t035.GetSysid()).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("history rows for removed T035 = %d (%v)", hist, err)
	}
}

func TestEmployeeComponentsScopeAndValidation(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	mustCreate(t, "LC002", "Scope Uji")
	if _, err := handler.GetEmployeeComponents(ctxFor(t, userJkt, permsRW...), &payrollv1.GetEmployeeComponentsRequest{Nik: "LC002", CompanyId: 1}); code(t, err) != apperr.CodeNotFound {
		t.Fatalf("other division = %v, want NOT_FOUND", err)
	}
	if _, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "LC002", CompanyId: 1, Division: "SOLO",
		Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: 999999}}}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("unknown component = %v, want VALIDATION", err)
	}
	c := mustComponent(t, "T9B2", "T", "Validasi")
	if _, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "LC002", CompanyId: 1, Division: "SOLO",
		Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: c.GetSysid(), Amount: "1.1234567"}}}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("7 decimals = %v, want VALIDATION", err)
	}
	if _, err := handler.PreviewLevelChange(ctx, &payrollv1.PreviewLevelChangeRequest{Nik: "LC002", CompanyId: 1, Division: "SOLO", NewLevel: "NO-SUCH", EffectiveYymm: 202610}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("unknown level = %v, want VALIDATION", err)
	}
}

func TestPutComponents_EditedTemplateRowBecomesManualAndRetiredStaysEditable(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	c := mustComponent(t, "T9C1", "T", "Edit template")
	putLevelTemplate(t, "LV-PUT", c.GetSysid())
	in := input("Put Uji")
	in.Level = "LV-PUT"
	if _, err := handler.CreateEmployee(ctx, &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "SOLO", Nik: "LC003", Input: in}); err != nil {
		t.Fatal(err)
	}
	key := &payrollv1.GetEmployeeComponentsRequest{Nik: "LC003", CompanyId: 1, Division: "SOLO"}
	got, _ := handler.GetEmployeeComponents(ctx, key)
	if m := codesOf(got.GetItems()); m["T9C1"].GetSource() != "TEMPLATE" {
		t.Fatalf("created = %v", m)
	}
	put := func(amount string) error {
		_, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "LC003", CompanyId: 1, Division: "SOLO",
			Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: c.GetSysid(), Sts: "F", Amount: amount}}})
		return err
	}
	// Unchanged round trip keeps TEMPLATE; an edited amount flips the row to MANUAL.
	if err := put("100"); err != nil {
		t.Fatal(err)
	}
	got, _ = handler.GetEmployeeComponents(ctx, key)
	if codesOf(got.GetItems())["T9C1"].GetSource() != "TEMPLATE" {
		t.Fatal("unchanged PUT changed the source")
	}
	if err := put("250"); err != nil {
		t.Fatal(err)
	}
	got, _ = handler.GetEmployeeComponents(ctx, key)
	if r := codesOf(got.GetItems())["T9C1"]; r.GetSource() != "MANUAL" || r.GetAmount() != "250" {
		t.Fatalf("edited row = %+v", r)
	}
	var hist int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_employee_component_hist
	  WHERE pec_nik='LC003' AND pec_component_sysid=$1`, c.GetSysid()).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("hist rows = %d (%v)", hist, err)
	}
	// A retired component that is already attached stays editable (GET -> PUT round trip).
	if _, err := handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: c.GetSysid(), IfMatch: c.GetEtag()}); err != nil {
		t.Fatal(err)
	}
	if err := put("300"); err != nil {
		t.Fatalf("PUT of retired attached component: %v", err)
	}
	// Level cannot change through UpdateEmployee (07§1); use level-change.
	emp, _ := handler.GetEmployee(ctx, &payrollv1.GetEmployeeRequest{Nik: "LC003", CompanyId: 1, Division: "SOLO"})
	in.Level = "LV-OTHER"
	if _, err := handler.UpdateEmployee(ctx, &payrollv1.UpdateEmployeeRequest{Id: emp.GetData().GetId(), Input: in, IfMatch: emp.GetData().GetEtag()}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("level via update = %v, want VALIDATION", err)
	}
}

func TestPutComponents_ClosedPeriodRefused(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	mustCreate(t, "LC004", "Closed Uji")
	c := mustComponent(t, "T9C2", "T", "Closed")
	if _, err := ownDB.ExecContext(context.Background(), `INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status)
	  VALUES (1,202511,'SOLO','2025-10-26','2025-11-25','CLOSED') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	_, err := handler.PutEmployeeComponents(ctx, &payrollv1.PutEmployeeComponentsRequest{Nik: "LC004", CompanyId: 1, Division: "SOLO",
		Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: c.GetSysid(), Amount: "1", ValidFrom: 202511}}})
	if code(t, err) != apperr.CodePeriodClosed {
		t.Fatalf("put into CLOSED period = %v, want PERIOD_CLOSED", err)
	}
}
