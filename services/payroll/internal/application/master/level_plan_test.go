package master

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

func entry(sysid int64, code string) component.CatalogEntry {
	return component.CatalogEntry{Sysid: sysid, Code: code, Name: code, DefaultSts: "F"}
}

func testCatalog() map[string]component.CatalogEntry {
	return map[string]component.CatalogEntry{
		"T002": entry(2, "T002"), "T003": entry(3, "T003"), "T004": entry(4, "T004"),
		"T006": entry(6, "T006"), "T034": entry(34, "T034"), "T035": entry(35, "T035"), "T036": entry(36, "T036"),
	}
}

func att(sysid int64, code, amount, source string, frz int) component.Attachment {
	return component.Attachment{Sysid: sysid, Code: code, Name: code, Sts: "F", Amount: amount, Source: source, Frz: frz}
}

func actions(p levelPlan) map[string]string {
	out := map[string]string{}
	for _, c := range p.Changes {
		out[c.Code] = c.Action
	}
	return out
}

func TestPlanLevel_NonStaffReplacesTemplateAttachments(t *testing.T) {
	template := []component.Attachment{att(2, "T002", "100", "", 2), att(3, "T003", "50", "", 2), att(36, "T036", "", "", 2)}
	rule := &component.LevelRule{Level: "SHIFT-I", Group: "SHIFT", OtinworkRate: "0.0877", OtinworkSts: 2, ShiftadjSts: 1,
		TunjshiftSts: 2, TunjshiftRate: "2200"}
	want, err := desiredFromTemplate(template, rule, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	// Employee came from a staff level: holds T2/T3 + T35 as TEMPLATE, and T4 as MANUAL.
	current := []component.Attachment{
		att(2, "T002", "100", component.SourceTemplate, 2), att(3, "T003", "50", component.SourceTemplate, 2),
		att(35, "T035", "3300", component.SourceTemplate, 1), att(4, "T004", "10", component.SourceManual, 2),
	}
	got := actions(planLevel(current, want, 202610))
	exp := map[string]string{"T002": "REMOVE", "T003": "REMOVE", "T035": "REMOVE", "T006": "ADD", "T034": "ADD", "T036": "ADD"}
	if len(got) != len(exp) {
		t.Fatalf("changes = %v, want %v", got, exp)
	}
	for k, v := range exp {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestPlanLevel_ManualIsNeverTouched(t *testing.T) {
	template := []component.Attachment{att(2, "T002", "100", "", 2)}
	want, _ := desiredFromTemplate(template, nil, testCatalog()) //nolint:errcheck // nil rule cannot fail
	current := []component.Attachment{att(2, "T002", "999", component.SourceManual, 2), att(4, "T004", "10", component.SourceManual, 2)}
	p := planLevel(current, want, 202610)
	if len(p.Changes) != 0 {
		t.Fatalf("manual rows changed: %+v", p.Changes)
	}
	if len(p.SkippedManual) != 1 || p.SkippedManual[0] != "T002" {
		t.Fatalf("skipped = %v, want [T002]", p.SkippedManual)
	}
}

func TestPlanLevel_StaffSwapsShiftComponentsAndChangesRate(t *testing.T) {
	rule := &component.LevelRule{Level: "STAFF-HO", Group: "STAFF", ShiftadjSts: 1, TunjshiftSts: 1, TunjshiftRate: "3300"}
	want, err := desiredFromTemplate([]component.Attachment{att(2, "T002", "100", "", 2)}, rule, testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	current := []component.Attachment{
		att(2, "T002", "80", component.SourceTemplate, 2), att(6, "T006", "0.0877", component.SourceTemplate, 2),
		att(34, "T034", "2200", component.SourceTemplate, 2),
	}
	got := actions(planLevel(current, want, 202610))
	exp := map[string]string{"T002": "CHANGE", "T006": "REMOVE", "T034": "REMOVE", "T035": "ADD"}
	for k, v := range exp {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(exp) {
		t.Fatalf("changes = %v, want %v", got, exp)
	}
}

func TestDesiredFromTemplate_MissingRuleComponentIsValidationError(t *testing.T) {
	rule := &component.LevelRule{Group: "SHIFT", OtinworkRate: "0.1", OtinworkSts: 2, TunjshiftSts: 2, TunjshiftRate: "1"}
	if _, err := desiredFromTemplate(nil, rule, map[string]component.CatalogEntry{}); err == nil {
		t.Fatal("expected error when T006/T034 are not in the catalog")
	}
}

func TestPlanLevel_IdenticalAttachmentsNoChange(t *testing.T) {
	want, _ := desiredFromTemplate([]component.Attachment{att(2, "T002", "100", "", 2)}, nil, testCatalog()) //nolint:errcheck // nil rule
	current := []component.Attachment{att(2, "T002", "100.000", component.SourceTemplate, 2)}
	if p := planLevel(current, want, 202610); len(p.Changes) != 0 {
		t.Fatalf("changes = %+v, want none (100 == 100.000)", p.Changes)
	}
}

// D-21: CreateEmployee is lenient. Rule components the company lacks are skipped and reported,
// level-change (strict) still fails on them.
func TestDesire_LenientSkipsMissingRuleComponents(t *testing.T) {
	rule := &component.LevelRule{Level: "SHIFT-I", Group: "SHIFT", OtinworkRate: "0.0877", OtinworkSts: 2, TunjshiftSts: 2, TunjshiftRate: "2200"}
	template := []component.Attachment{att(2, "T002", "100", "", 2)}
	catalog := map[string]component.CatalogEntry{"T034": entry(34, "T034")} // T006 absent
	want, missing, err := desire(template, rule, catalog, true)
	if err != nil {
		t.Fatalf("lenient: %v", err)
	}
	if len(missing) != 1 || missing[0] != "T006" {
		t.Fatalf("missing = %v, want [T006]", missing)
	}
	if _, ok := want[34]; !ok {
		t.Fatalf("T034 not attached: %v", want)
	}
	if _, _, err := desire(template, rule, catalog, false); err == nil {
		t.Fatal("strict mode must fail on a missing rule component")
	}
	// Empty catalog (new company): nothing fails, nothing from the rule is attached.
	if _, missing, err = desire(nil, rule, map[string]component.CatalogEntry{}, true); err != nil || len(missing) != 2 {
		t.Fatalf("empty catalog: missing=%v err=%v", missing, err)
	}
}

// Staff levels (T35) skip too when the company lacks the component.
func TestDesire_LenientStaffMissingT035(t *testing.T) {
	rule := &component.LevelRule{Level: "STAFF", Group: "STAFF", TunjshiftSts: 1, TunjshiftRate: "3300"}
	_, missing, err := desire(nil, rule, map[string]component.CatalogEntry{}, true)
	if err != nil || len(missing) != 1 || missing[0] != "T035" {
		t.Fatalf("missing=%v err=%v", missing, err)
	}
}
