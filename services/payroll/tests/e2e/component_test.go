package e2e

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"testing"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/apperr"
)

func compInput(name string) *payrollv1.ComponentInput {
	return &payrollv1.ComponentInput{
		Name: name, DefaultSts: "F", TaxClass: "TAXABLE", IncludeInEstimate: true,
		Tags: []string{"bpjs_base"}, Serial: 10, ValidFrom: 202601,
	}
}

func mustComponent(t *testing.T, code, typ, name string) *payrollv1.Component {
	t.Helper()
	resp, err := handler.CreateComponent(ctxFor(t, userRW, permsRW...), &payrollv1.CreateComponentRequest{
		CompanyId: 1, Code: code, Type: typ, Input: compInput(name),
	})
	if err != nil {
		t.Fatalf("create component %s: %v", code, err)
	}
	return resp.GetData()
}

func TestComponentCRUDRetireAndAudit(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	c := mustComponent(t, "T901", "T", "Tunjangan Uji")
	if c.GetSysid() == 0 || c.GetStatus() != "ACTIVE" || c.GetEtag() != "1" || c.GetType() != "T" ||
		len(c.GetTags()) != 1 || c.GetValidTo() != 0 {
		t.Fatalf("unexpected created component: %+v", c)
	}

	got, err := handler.GetComponent(ctx, &payrollv1.GetComponentRequest{Sysid: c.GetSysid()})
	if err != nil || got.GetData().GetCode() != "T901" {
		t.Fatalf("get: %v %+v", err, got)
	}

	// Duplicate (company, code) is a conflict.
	_, err = handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T901", Type: "T", Input: compInput("Dobel")})
	if code(t, err) != apperr.CodeConflict {
		t.Fatalf("duplicate code = %v, want CONFLICT", err)
	}

	// Update bumps the etag; a stale If-Match is 412.
	in := compInput("Tunjangan Uji 2")
	in.ScopeDivisions = []string{"SOLO"}
	up, err := handler.UpdateComponent(ctx, &payrollv1.UpdateComponentRequest{Sysid: c.GetSysid(), Input: in, IfMatch: "1"})
	if err != nil || up.GetData().GetEtag() != "2" || up.GetData().GetName() != "Tunjangan Uji 2" ||
		len(up.GetData().GetScopeDivisions()) != 1 {
		t.Fatalf("update: %v %+v", err, up)
	}
	_, err = handler.UpdateComponent(ctx, &payrollv1.UpdateComponentRequest{Sysid: c.GetSysid(), Input: in, IfMatch: "1"})
	if code(t, err) != apperr.CodeVersionMismatch {
		t.Fatalf("stale if-match = %v, want VERSION_MISMATCH", err)
	}

	// Retire: status RETIRED, further edits and a second retire are refused; the row stays.
	ret, err := handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: c.GetSysid(), IfMatch: "2"})
	if err != nil || ret.GetData().GetStatus() != "RETIRED" || ret.GetData().GetEtag() != "3" {
		t.Fatalf("retire: %v %+v", err, ret)
	}
	_, err = handler.UpdateComponent(ctx, &payrollv1.UpdateComponentRequest{Sysid: c.GetSysid(), Input: in, IfMatch: "3"})
	if code(t, err) != apperr.CodeConflict {
		t.Fatalf("update retired = %v, want CONFLICT", err)
	}
	_, err = handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: c.GetSysid(), IfMatch: "3"})
	if code(t, err) != apperr.CodeConflict {
		t.Fatalf("retire twice = %v, want CONFLICT", err)
	}

	var n int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_audit_log
	  WHERE pal_entity='pyr_component' AND pal_entity_key=$1`, strconv.FormatInt(c.GetSysid(), 10)).Scan(&n); err != nil || n != 3 {
		t.Fatalf("audit rows = %d (%v), want CREATE+UPDATE+RETIRE", n, err)
	}
}

func TestComponentValidation(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	bad := compInput("Divisi Hantu")
	bad.ScopeDivisions = []string{"BANDUNG"}
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T902", Type: "T", Input: bad}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("unknown division = %v, want VALIDATION", err)
	}
	bad = compInput("Periode Terbalik")
	bad.ValidFrom, bad.ValidTo = 202610, 202601
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T903", Type: "T", Input: bad}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("valid_to < valid_from = %v, want VALIDATION", err)
	}
	bad = compInput("Yymm Salah")
	bad.ValidFrom = 202613
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T904", Type: "T", Input: bad}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("month 13 = %v, want VALIDATION", err)
	}
	bad = compInput("Pasangan Hantu")
	bad.BaseComponentSysid = 999999
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T905", Type: "T", Input: bad}); code(t, err) != apperr.CodeValidation {
		t.Fatalf("unknown base component = %v, want VALIDATION", err)
	}
	// A valid pair works: base points at an existing same-company component.
	base := mustComponent(t, "T906", "T", "Dasar")
	pair := compInput("Pasangan")
	pair.BaseComponentSysid = base.GetSysid()
	if r, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T907", Type: "T", Input: pair}); err != nil || r.GetData().GetBaseComponentSysid() != base.GetSysid() {
		t.Fatalf("pair create: %v %+v", err, r)
	}
}

func TestComponentScopeAndList(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	// A second company the SOLO user has no scope in.
	var co2 int
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_company (pco_code, pco_name)
	  VALUES ('CMP2','Company Dua') RETURNING pco_company_id`).Scan(&co2); err != nil {
		t.Fatal(err)
	}
	var foreign int64
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from)
	  VALUES ($1,'X001','Tunjangan','Asing',202601) RETURNING pc_sysid`, co2).Scan(&foreign); err != nil {
		t.Fatal(err)
	}

	if _, err := handler.GetComponent(ctx, &payrollv1.GetComponentRequest{Sysid: foreign}); code(t, err) != apperr.CodeNotFound {
		t.Fatalf("foreign get = %v, want NOT_FOUND (RLS)", err)
	}
	if _, err := handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: int32(co2), Code: "X002", Type: "T", Input: compInput("Asing 2")}); code(t, err) != apperr.CodeDivisionForbidden {
		t.Fatalf("foreign create = %v, want DIVISION_FORBIDDEN", err)
	}
	if _, err := handler.UpdateComponent(ctx, &payrollv1.UpdateComponentRequest{Sysid: foreign, Input: compInput("x"), IfMatch: "1"}); code(t, err) != apperr.CodeNotFound {
		t.Fatalf("foreign update = %v, want NOT_FOUND", err)
	}
	if _, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{CompanyId: int32(co2)}); code(t, err) != apperr.CodeDivisionForbidden {
		t.Fatalf("foreign list = %v, want DIVISION_FORBIDDEN", err)
	}

	// Own list never includes the foreign component, supports filters and a keyset cursor.
	mustComponent(t, "P910", "P", "Potongan Uji")
	all, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all.GetData() {
		if c.GetSysid() == foreign {
			t.Fatal("list leaked a component of another company")
		}
	}
	onlyP, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{Type: "P", Q: "potongan uji"})
	if err != nil || len(onlyP.GetData()) != 1 || onlyP.GetData()[0].GetCode() != "P910" {
		t.Fatalf("filter: %v %+v", err, onlyP)
	}
	page1, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{Limit: 1})
	if err != nil || len(page1.GetData()) != 1 || page1.GetNextCursor() == "" {
		t.Fatalf("page1: %v %+v", err, page1)
	}
	page2, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{Limit: 1, Cursor: page1.GetNextCursor()})
	if err != nil || len(page2.GetData()) != 1 || page2.GetData()[0].GetSysid() <= page1.GetData()[0].GetSysid() {
		t.Fatalf("page2: %v %+v", err, page2)
	}
	// LIKE wildcards in q are literal.
	if r, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{Q: "%"}); err != nil || len(r.GetData()) != 0 {
		t.Fatalf("wildcard q: %v %+v", err, r)
	}
}

func TestComponentTemplate(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	a := mustComponent(t, "T920", "T", "Tmpl A")
	b := mustComponent(t, "P921", "P", "Tmpl B")
	retired := mustComponent(t, "T922", "T", "Tmpl Retired")
	if _, err := handler.RetireComponent(ctx, &payrollv1.RetireComponentRequest{Sysid: retired.GetSysid(), IfMatch: "1"}); err != nil {
		t.Fatal(err)
	}

	put, err := handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-T008", Items: []*payrollv1.TemplateItem{
		{ComponentSysid: a.GetSysid(), Sts: "F", Tax: 2, Amount: "0.0877"},
		{ComponentSysid: b.GetSysid(), Amount: "2200"},
	}})
	if err != nil || len(put.GetItems()) != 2 || put.GetItems()[0].GetComponentCode() != "T920" || put.GetItems()[0].GetAmount() != "0.0877" {
		t.Fatalf("put: %v %+v", err, put)
	}

	// Replace: whole template swapped.
	if _, err := handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-T008", Items: []*payrollv1.TemplateItem{
		{ComponentSysid: b.GetSysid(), Sts: "D", Amount: "3300.50"},
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := handler.GetComponentTemplate(ctx, &payrollv1.GetComponentTemplateRequest{Level: "LVL-T008"})
	if err != nil || len(got.GetItems()) != 1 || got.GetItems()[0].GetSts() != "D" || got.GetItems()[0].GetAmount() != "3300.5" {
		t.Fatalf("get after replace: %v %+v", err, got)
	}

	for name, items := range map[string][]*payrollv1.TemplateItem{
		"unknown":   {{ComponentSysid: 999999}},
		"retired":   {{ComponentSysid: retired.GetSysid()}},
		"duplicate": {{ComponentSysid: a.GetSysid()}, {ComponentSysid: a.GetSysid()}},
		"not-a-num": {{ComponentSysid: a.GetSysid(), Amount: "1e999999"}},
		"float-txt": {{ComponentSysid: a.GetSysid(), Amount: "abc"}},
	} {
		if _, err := handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-T008", Items: items}); code(t, err) != apperr.CodeValidation {
			t.Fatalf("%s: %v, want VALIDATION", name, err)
		}
	}
	// A rejected put leaves the template untouched.
	if got, _ := handler.GetComponentTemplate(ctx, &payrollv1.GetComponentTemplateRequest{Level: "LVL-T008"}); len(got.GetItems()) != 1 {
		t.Fatalf("template changed by a rejected put: %+v", got)
	}

	// D-20: a user without scope in the component's company neither sees nor wipes its template rows.
	var co2 int
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_company (pco_code, pco_name)
	  VALUES ('CMP3','Company Tiga') RETURNING pco_company_id`).Scan(&co2); err != nil {
		t.Fatal(err)
	}
	var foreign int64
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from)
	  VALUES ($1,'Y001','Tunjangan','Asing',202601) RETURNING pc_sysid`, co2).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := ownDB.ExecContext(context.Background(), `INSERT INTO payroll.pyr_component_template (pct_template, pct_component_sysid, pct_amount)
	  VALUES ('LVL-T008',$1,1)`, foreign); err != nil {
		t.Fatal(err)
	}
	got, _ = handler.GetComponentTemplate(ctx, &payrollv1.GetComponentTemplateRequest{Level: "LVL-T008"})
	for _, it := range got.GetItems() {
		if it.GetComponentSysid() == foreign {
			t.Fatal("template read leaked a row of another company")
		}
	}
	if _, err := handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-T008", Items: nil}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_component_template
	  WHERE pct_template='LVL-T008' AND pct_component_sysid=$1`, foreign).Scan(&n); err != nil || n != 1 {
		t.Fatalf("foreign template row count = %d (%v), want 1 (untouched)", n, err)
	}
}

func TestComponentTemplateGuards(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	a := mustComponent(t, "T930", "T", "Guard A")

	// More than 6 decimals would be rounded silently by numeric(18,6): rejected.
	_, err := handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-G", Items: []*payrollv1.TemplateItem{
		{ComponentSysid: a.GetSysid(), Amount: "0.12345678"}}})
	if code(t, err) != apperr.CodeValidation {
		t.Fatalf("7+ decimals = %v, want VALIDATION", err)
	}

	// Commas/spaces would be split by the CSV array transport: rejected before SQL.
	in := compInput("Koma")
	in.ScopeDivisions = []string{"SOLO,JAKARTA"}
	_, err = handler.CreateComponent(ctx, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T931", Type: "T", Input: in})
	if code(t, err) != apperr.CodeValidation {
		t.Fatalf("comma in division = %v, want VALIDATION", err)
	}

	// A component of a company outside the caller's scope is invisible: PUT is VALIDATION, never a write.
	var co2 int
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_company (pco_code, pco_name)
	  VALUES ('CMP4','Company Empat') RETURNING pco_company_id`).Scan(&co2); err != nil {
		t.Fatal(err)
	}
	var foreign int64
	if err := ownDB.QueryRowContext(context.Background(), `INSERT INTO payroll.pyr_component (pc_company_id, pc_code, pc_type, pc_name, pc_valid_from)
	  VALUES ($1,'Z001','Tunjangan','Asing',202601) RETURNING pc_sysid`, co2).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	_, err = handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-G", Items: []*payrollv1.TemplateItem{
		{ComponentSysid: foreign}}})
	if code(t, err) != apperr.CodeValidation {
		t.Fatalf("foreign component in PUT = %v, want VALIDATION", err)
	}
	var n int
	if err := ownDB.QueryRowContext(context.Background(), `SELECT count(*) FROM payroll.pyr_component_template WHERE pct_component_sysid=$1`, foreign).Scan(&n); err != nil || n != 0 {
		t.Fatalf("foreign component got a template row: %d (%v)", n, err)
	}

	// Template table itself is RLS-scoped: even a raw SELECT as payroll_app sees only own-scope rows (D-20).
	if _, err := ownDB.ExecContext(context.Background(), `INSERT INTO payroll.pyr_component_template (pct_template, pct_component_sysid, pct_amount) VALUES ('LVL-RAW',$1,1)`, foreign); err != nil {
		t.Fatal(err)
	}
	err = appDB.ScopedTx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_component_template WHERE pct_template='LVL-RAW'`).Scan(&n)
	})
	if err != nil || n != 0 {
		t.Fatalf("raw template select saw %d foreign rows (%v), want 0", n, err)
	}
}

// Two PUTs of one level at once serialize on the advisory lock: both succeed, last wins, no PK error.
func TestComponentTemplateConcurrentPut(t *testing.T) {
	ctx := ctxFor(t, userRW, permsRW...)
	a := mustComponent(t, "T940", "T", "Conc A")
	b := mustComponent(t, "T941", "T", "Conc B")
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sysid := a.GetSysid()
			if i%2 == 1 {
				sysid = b.GetSysid()
			}
			_, errs[i] = handler.PutComponentTemplate(ctx, &payrollv1.PutComponentTemplateRequest{Level: "LVL-CONC",
				Items: []*payrollv1.TemplateItem{{ComponentSysid: sysid, Amount: "1"}}})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent put %d: %v", i, err)
		}
	}
	got, err := handler.GetComponentTemplate(ctx, &payrollv1.GetComponentTemplateRequest{Level: "LVL-CONC"})
	if err != nil || len(got.GetItems()) != 1 {
		t.Fatalf("after concurrent puts: %v %+v", err, got)
	}
}

func TestComponentRPCsRequirePermission(t *testing.T) {
	// The permission map is exercised at interceptor level in delivery tests; here the use case still
	// refuses a user whose scope does not cover the component's company even with every permission.
	ctx := ctxFor(t, userJkt, permsRW...) // JAKARTA scope, same company: allowed to see company-level data
	if _, err := handler.ListComponents(ctx, &payrollv1.ListComponentsRequest{}); err != nil {
		t.Fatalf("company-level list for JAKARTA user: %v", err)
	}
	empty := ctxFor(t, "user-nobody", permsRW...)
	if _, err := handler.CreateComponent(empty, &payrollv1.CreateComponentRequest{CompanyId: 1, Code: "T950", Type: "T", Input: compInput("Nobody")}); code(t, err) != apperr.CodeDivisionForbidden {
		t.Fatalf("no-scope create = %v, want DIVISION_FORBIDDEN", err)
	}
}
