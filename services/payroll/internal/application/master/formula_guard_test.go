package master

import (
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

func TestFormulaMapped(t *testing.T) {
	cases := []struct {
		typ, code string
		want      bool
	}{
		{"T", "T001", true},
		{"T", "T037", true}, // LEGACY_NULL allowlist
		{"T", "T080", false},
		{"P", "P001", true},
		{"P", "P099", false},
		{"P", "P008", true}, // LEGACY_NULL allowlist
		{"p", "p001", true},
		{"T", "T", false},
		{"T", "Txyz", false},
	}
	for _, c := range cases {
		if got := formulaMapped(c.typ, c.code); got != c.want {
			t.Errorf("formulaMapped(%q,%q) = %v, want %v", c.typ, c.code, got, c.want)
		}
	}
}

func TestFormulaWarning(t *testing.T) {
	if w := FormulaWarning(component.Component{Type: "T", Code: "T001"}); w != "" {
		t.Errorf("mapped component warned: %q", w)
	}
	if w := FormulaWarning(component.Component{Type: "T", Code: "T080"}); !strings.Contains(w, "T080") {
		t.Errorf("unmapped component: warning %q", w)
	}
}

func TestResolvePut_UnmappedComponent(t *testing.T) {
	cat := component.CatalogEntry{Sysid: 80, Code: "T080", Name: "Baru", DefaultSts: "F", TaxClass: component.TaxClassTaxable}
	in := component.Attachment{Sysid: 80, Frz: 2}
	if _, err := resolvePut(in, component.Attachment{}, false, cat, true); err == nil ||
		!strings.Contains(err.Error(), "belum ada formula di engine") {
		t.Fatalf("new attach of unmapped component: err = %v", err)
	}
	held := component.Attachment{Sysid: 80, Code: "T080", Name: "Baru", Sts: "F", Tax: component.TaxTaxable, Frz: 2, Source: component.SourceManual}
	if _, err := resolvePut(in, held, true, cat, true); err != nil {
		t.Fatalf("already attached stays editable: %v", err)
	}
	cat.Code = "T001"
	if _, err := resolvePut(in, component.Attachment{}, false, cat, true); err != nil {
		t.Fatalf("mapped component: %v", err)
	}
}

func TestGuardNewTemplateItems(t *testing.T) {
	before := []component.TemplateItem{{Sysid: 80, Code: "T080"}}
	if err := guardNewTemplateItems(before, map[int64]string{80: "T080"}); err != nil {
		t.Errorf("held row must pass: %v", err)
	}
	err := guardNewTemplateItems(before, map[int64]string{81: "T081"})
	if err == nil || !strings.Contains(err.Error(), "T081") {
		t.Errorf("new unmapped row: err = %v", err)
	}
}

func TestGuardPlanAdds(t *testing.T) {
	mixed := levelPlan{Changes: []component.Change{
		{Action: component.ActionAdd, Code: "T001"},
		{Action: component.ActionRemove, Code: "T080"},
		{Action: component.ActionAdd, Code: "T080"},
	}}
	if _, err := guardPlanAdds(mixed, "L1", false); err == nil || !strings.Contains(err.Error(), "T080") {
		t.Errorf("strict: unmapped add must fail naming the code, err = %v", err)
	}
	got, err := guardPlanAdds(mixed, "L1", true)
	if err != nil {
		t.Fatalf("lenient must not fail: %v", err)
	}
	if len(got.Changes) != 2 || got.Changes[0].Code != "T001" || got.Changes[1].Action != component.ActionRemove {
		t.Errorf("lenient changes = %+v", got.Changes)
	}
	if len(got.Missing) != 1 || got.Missing[0] != "T080" {
		t.Errorf("lenient Missing = %v, want [T080]", got.Missing)
	}
	ok := levelPlan{Changes: []component.Change{{Action: component.ActionAdd, Code: "T001"}, {Action: component.ActionRemove, Code: "T080"}}}
	if got, err := guardPlanAdds(ok, "L1", false); err != nil || len(got.Changes) != 2 {
		t.Errorf("mapped add / unmapped remove: %v %+v", err, got)
	}
}

func TestValidateComponentCodeVsType(t *testing.T) {
	base := component.Component{ValidFrom: 202601}
	cases := []struct {
		typ, code string
		ok        bool
	}{
		{"T", "T001", true},
		{"P", "P032", true},
		{"P", "T001", false}, // type P with a T code
		{"T", "P001", false},
		{"T", "Txyz", false},
		{"T", "T", false},
		{"T", "", false},
	}
	for _, c := range cases {
		comp := base
		comp.Type, comp.Code = c.typ, c.code
		if err := validateComponent(comp); (err == nil) != c.ok {
			t.Errorf("validateComponent(%q,%q) err = %v, want ok=%v", c.typ, c.code, err, c.ok)
		}
	}
}
