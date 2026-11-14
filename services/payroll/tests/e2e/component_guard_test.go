package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

func updateComponent(t *testing.T, c *payrollv1.Component, mutate func(*payrollv1.ComponentInput)) error {
	t.Helper()
	cur, err := handler.GetComponent(ctxFor(t, userRW, permsRW...), &payrollv1.GetComponentRequest{Sysid: c.GetSysid()})
	if err != nil {
		t.Fatalf("get component: %v", err)
	}
	in := compInput(cur.GetData().GetName())
	in.Name = cur.GetData().GetName()
	in.TaxClass, in.Currency, in.ValidFrom, in.ValidTo = cur.GetData().GetTaxClass(), cur.GetData().GetCurrency(), cur.GetData().GetValidFrom(), cur.GetData().GetValidTo()
	in.ScopeDivisions, in.BaseComponentSysid = cur.GetData().GetScopeDivisions(), cur.GetData().GetBaseComponentSysid()
	mutate(in)
	_, err = handler.UpdateComponent(ctxFor(t, userRW, permsRW...), &payrollv1.UpdateComponentRequest{Sysid: c.GetSysid(), Input: in, IfMatch: cur.GetData().GetEtag()})
	return err
}

// (1) RETIRED components are not copied by CreateEmployee/level-change and vanish from the template view.
func TestRetiredComponentNotAttachedAndHiddenFromTemplate(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	live := mustComponent(t, "T9R1", "T", "Retire live")
	gone := mustComponent(t, "T9R2", "T", "Retire gone")
	putLevelTemplate(t, "LV-RET", live.GetSysid(), gone.GetSysid())
	cur, err := handler.GetComponent(ctx, &payrollv1.GetComponentRequest{Sysid: gone.GetSysid()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: gone.GetSysid(), IfMatch: cur.GetData().GetEtag()}); err != nil {
		t.Fatalf("retire: %v", err)
	}
	tpl, err := handler.GetComponentTemplate(ctx, &payrollv1.GetComponentTemplateRequest{Level: "LV-RET"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range tpl.GetItems() {
		if it.GetComponentSysid() == gone.GetSysid() {
			t.Fatalf("RETIRED component still in template view: %v", tpl.GetItems())
		}
	}
	createAt(t, "RT001", "LV-RET", "2026-05-10")
	if got := componentsOf(t, "RT001"); got["T9R2"] != nil || got["T9R1"] == nil {
		t.Fatalf("attachments = %v, want T9R1 only", got)
	}
}

// (2) base_component_sysid cycles and a RETIRED base are refused.
func TestComponentBaseCycleAndRetiredBase(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	a := mustComponent(t, "T9K1", "T", "Base A")
	b := mustComponent(t, "T9K2", "T", "Base B")
	c3 := mustComponent(t, "T9K3", "T", "Base C")
	if err := updateComponent(t, b, func(in *payrollv1.ComponentInput) { in.BaseComponentSysid = a.GetSysid() }); err != nil {
		t.Fatalf("B->A: %v", err)
	}
	if err := updateComponent(t, a, func(in *payrollv1.ComponentInput) { in.BaseComponentSysid = b.GetSysid() }); code(t, err) != apperr.CodeValidation {
		t.Fatalf("A<->B = %v, want VALIDATION", err)
	}
	if err := updateComponent(t, c3, func(in *payrollv1.ComponentInput) { in.BaseComponentSysid = b.GetSysid() }); err != nil {
		t.Fatalf("C->B: %v", err)
	}
	if err := updateComponent(t, a, func(in *payrollv1.ComponentInput) { in.BaseComponentSysid = c3.GetSysid() }); code(t, err) != apperr.CodeValidation {
		t.Fatalf("A->C->B->A = %v, want VALIDATION", err)
	}
	// RETIRED base: refused on create and on a new link, but an unchanged link stays editable.
	cur, _ := handler.GetComponent(ctx, &payrollv1.GetComponentRequest{Sysid: a.GetSysid()}) //nolint:errcheck // just created
	if _, err := handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: a.GetSysid(), IfMatch: cur.GetData().GetEtag()}); err != nil {
		t.Fatal(err)
	}
	in := compInput("Pakai base RETIRED")
	in.BaseComponentSysid = a.GetSysid()
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T9K4", Type: "T", Input: in}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("create with RETIRED base = %v, want VALIDATION", err)
	}
	if err := updateComponent(t, b, func(in *payrollv1.ComponentInput) { in.Name = "B renamed" }); err != nil {
		t.Fatalf("unchanged RETIRED base must stay editable: %v", err)
	}
	if err := updateComponent(t, c3, func(in *payrollv1.ComponentInput) { in.BaseComponentSysid = a.GetSysid() }); code(t, err) != apperr.CodeValidation {
		t.Fatalf("new link to RETIRED base = %v, want VALIDATION", err)
	}
}

// (3) Guarded fields of a component attached to anyone (any division) are frozen; free fields are not.
func TestComponentGuardedFieldsWhenUsed(t *testing.T) {
	c := mustComponent(t, "T9U1", "T", "Used guard")
	if err := updateComponent(t, c, func(in *payrollv1.ComponentInput) { in.TaxClass = "NONTAX" }); err != nil {
		t.Fatalf("unused component must allow tax_class change: %v", err)
	}
	mustCreate(t, "UG001", "Used Guard")
	if _, err := handler.PutEmployeeComponents(ctxFor(t, userRW, permsRW...), &payrollv1.PutEmployeeComponentsRequest{Nik: "UG001", CompanyId: 1, Division: "SOLO",
		Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: c.GetSysid(), Amount: "1"}}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	for name, mutate := range map[string]func(*payrollv1.ComponentInput){
		"tax_class":       func(in *payrollv1.ComponentInput) { in.TaxClass = "TAXABLE" },
		"valid_from":      func(in *payrollv1.ComponentInput) { in.ValidFrom = 202602 },
		"valid_to":        func(in *payrollv1.ComponentInput) { in.ValidTo = 203012 },
		"scope_divisions": func(in *payrollv1.ComponentInput) { in.ScopeDivisions = []string{"SOLO"} },
	} {
		if err := updateComponent(t, c, mutate); code(t, err) != apperr.CodeConflict {
			t.Fatalf("%s change on used component = %v, want CONFLICT", name, err)
		}
	}
	if err := updateComponent(t, c, func(in *payrollv1.ComponentInput) { in.Name = "Used guard renamed"; in.Serial = 99 }); err != nil {
		t.Fatalf("name/serial are free: %v", err)
	}
	// Attached only in JAKARTA: a SOLO-only caller must still be guarded (usage is checked across divisions).
	other := mustComponent(t, "T9U2", "T", "Used in JKT")
	if _, err := handler.CreateEmployee(ctxFor(t, userBoth, permsRW...), &payrollv1.CreateEmployeeRequest{CompanyId: 1, Division: "JAKARTA", Nik: "UG002", Input: input("Jkt Guard")}); err != nil {
		t.Fatalf("create jkt employee: %v", err)
	}
	if _, err := handler.PutEmployeeComponents(ctxFor(t, userBoth, permsRW...), &payrollv1.PutEmployeeComponentsRequest{Nik: "UG002", CompanyId: 1, Division: "JAKARTA",
		Items: []*payrollv1.EmployeeComponentInput{{ComponentSysid: other.GetSysid(), Amount: "1"}}}); err != nil {
		t.Fatalf("attach jkt: %v", err)
	}
	if err := updateComponent(t, other, func(in *payrollv1.ComponentInput) { in.TaxClass = "NONTAX" }); code(t, err) != apperr.CodeConflict {
		t.Fatalf("component used only in JAKARTA = %v, want CONFLICT for a SOLO caller", err)
	}
}

// (3b) A component booked in a CLOSED run is frozen even without attachments.
func TestComponentGuardedFieldsWhenInClosedRun(t *testing.T) {
	bg := context.Background()
	c := mustComponent(t, "T9U3", "T", "Closed run guard")
	for _, s := range []string{
		`INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from) VALUES (9001,'guard',202601) ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_period (pp_company_id, pp_yymm, pp_division, pp_date_from, pp_date_to, pp_status) VALUES (1,203404,'SOLO','2034-03-26','2034-04-25','OPEN') ON CONFLICT DO NOTHING`,
		`INSERT INTO payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division, pas_source_batch_ids, pas_checksum) VALUES (1,203404,'SOLO','{1}','g')`,
		`INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id, pr_snapshot_id, pr_status)
		   SELECT 1,203404,'SOLO','2034-04-25',9001,max(pas_snapshot_id),'ACTIVE' FROM payroll.pyr_att_snapshot`,
	} {
		if _, err := ownDB.ExecContext(bg, s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() { // the CLOSED period would block level-change tests that run later
		for _, q := range []string{
			`UPDATE payroll.pyr_period SET pp_status='OPEN' WHERE pp_company_id=1 AND pp_yymm=203404 AND pp_division='SOLO'`,
			`DELETE FROM payroll.pyr_salary_detail WHERE psd_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_code='T9U3')`,
			`DELETE FROM payroll.pyr_run WHERE pr_company_id=1 AND pr_yymm=203404 AND pr_division='SOLO'`,
			`DELETE FROM payroll.pyr_att_snapshot WHERE pas_company_id=1 AND pas_yymm=203404`,
			`DELETE FROM payroll.pyr_period WHERE pp_company_id=1 AND pp_yymm=203404 AND pp_division='SOLO'`,
		} {
			if _, err := ownDB.ExecContext(bg, q); err != nil {
				t.Errorf("cleanup %.50s: %v", q, err)
			}
		}
	})
	if _, err := ownDB.ExecContext(bg, `INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_amount, psd_component_sysid)
	  SELECT max(pr_id),'ZZ','Tunjangan',1,1,$1 FROM payroll.pyr_run WHERE pr_yymm=203404`, c.GetSysid()); err != nil {
		t.Fatalf("seed detail: %v", err)
	}
	if err := updateComponent(t, c, func(in *payrollv1.ComponentInput) { in.TaxClass = "NONTAX" }); err != nil {
		t.Fatalf("OPEN period must not freeze the component: %v", err)
	}
	if _, err := ownDB.ExecContext(bg, `UPDATE payroll.pyr_period SET pp_status='CLOSED' WHERE pp_yymm=203404 AND pp_division='SOLO' AND pp_company_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := updateComponent(t, c, func(in *payrollv1.ComponentInput) { in.TaxClass = "TAXABLE" }); code(t, err) != apperr.CodeConflict {
		t.Fatalf("tax_class on component in CLOSED run = %v, want CONFLICT", err)
	}
}

// (4) Migration 000022 down/up with legacy data and an ambiguous legacy map, inside a rolled-back tx.
func TestMigration000022_BackfillAmbiguousMapAndDownUp(t *testing.T) {
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
	exec := func(q string) error { _, e := tx.ExecContext(bg, q); return e }
	must := func(q string) {
		t.Helper()
		if e := exec(q); e != nil {
			t.Fatalf("%.80s: %v", q, e)
		}
	}
	// Go back to the pre-000022 shape (down), then seed legacy rows and run up again.
	must(`DELETE FROM payroll.pyr_component_template`)
	must(`DELETE FROM payroll.pyr_employee_component_hist`)
	must(`DELETE FROM payroll.pyr_employee_component`)
	must(read("000023_employee_component_sysid.down.sql"))
	must(read("000022_component_sysid_template.down.sql"))
	must(`INSERT INTO payroll.pyr_component_master (pcm_type, pcm_id, pcm_name) VALUES ('Tunjangan', 9101, 'Legacy 9101') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from, pc_status) VALUES
	  (1,'L9101A','Tunjangan','A',202601,'ACTIVE'), (1,'L9101B','Tunjangan','B',202601,'ACTIVE')`)
	must(`INSERT INTO payroll.pyr_component_template (pct_template, pct_type, pct_id) VALUES ('LV-MIG','Tunjangan',9101)`)

	// Ambiguous map: two divisions map the same (type,id) to two different sysids → up must stop.
	must(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid) SELECT 1, 'Tunjangan', 9101, 'SOLO', pc_sysid FROM payroll.pyr_component WHERE pc_code='L9101A'`)
	must(`INSERT INTO payroll.pyr_component_legacy_map (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division, pclm_sysid) SELECT 1, 'Tunjangan', 9101, 'JAKARTA', pc_sysid FROM payroll.pyr_component WHERE pc_code='L9101B'`)
	must(`SAVEPOINT amb`)
	if err = exec(read("000022_component_sysid_template.up.sql")); err == nil {
		t.Fatal("up succeeded with an ambiguous legacy map; it must stop (D-20)")
	}
	must(`ROLLBACK TO SAVEPOINT amb`)

	// Unambiguous map: backfill by sysid, then the PK is (template, sysid).
	must(`DELETE FROM payroll.pyr_component_legacy_map WHERE pclm_legacy_id=9101 AND pclm_legacy_division='JAKARTA'`)
	must(read("000022_component_sysid_template.up.sql"))
	var n int
	if err = tx.QueryRowContext(bg, `SELECT count(*) FROM payroll.pyr_component_template t JOIN payroll.pyr_component c ON c.pc_sysid=t.pct_component_sysid
	  WHERE t.pct_template='LV-MIG' AND c.pc_code='L9101A'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("backfilled template rows = %d (%v), want 1", n, err)
	}
	// down → up repeats cleanly with a legacy (type,id) row present.
	must(read("000022_component_sysid_template.down.sql"))
	must(read("000022_component_sysid_template.up.sql"))
}
