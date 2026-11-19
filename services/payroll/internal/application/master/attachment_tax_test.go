package master

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// T033f: tax 0 on PUT takes the pc_tax_class default for a new attachment and keeps the stored flag otherwise.
func TestResolvePut_TaxDefault(t *testing.T) {
	cat := func(class string) component.CatalogEntry {
		return component.CatalogEntry{Sysid: 57, Code: "T057", Name: "Baru", DefaultSts: "F", TaxClass: class}
	}
	held := component.Attachment{Sysid: 57, Code: "T057", Name: "Baru", Sts: "F", Tax: component.TaxNone, Frz: 2, Source: component.SourceManual}
	cases := []struct {
		name     string
		in       component.Attachment
		isHeld   bool
		class    string
		wantTax  int
		wantPaid string
	}{
		{"new TAXABLE", component.Attachment{Sysid: 57, Frz: 2}, false, component.TaxClassTaxable, component.TaxTaxable, ""},
		{"new TAX_PAID_COMPANY", component.Attachment{Sysid: 57, Frz: 2}, false, component.TaxClassPaidCompany, component.TaxTaxable, "Y"},
		{"new NONE", component.Attachment{Sysid: 57, Frz: 2}, false, "NONE", component.TaxNone, ""},
		{"new explicit 1 wins", component.Attachment{Sysid: 57, Frz: 2, Tax: component.TaxNone}, false, component.TaxClassTaxable, component.TaxNone, ""},
		{"held keeps stored flag", component.Attachment{Sysid: 57, Frz: 2}, true, component.TaxClassTaxable, component.TaxNone, ""},
		{"held 1 → 2 explicit", component.Attachment{Sysid: 57, Frz: 2, Tax: component.TaxTaxable}, true, component.TaxClassTaxable, component.TaxTaxable, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolvePut(c.in, held, c.isHeld, cat(c.class), true)
			if err != nil {
				t.Fatal(err)
			}
			if got.Tax != c.wantTax || got.TaxPaidCompany != c.wantPaid {
				t.Errorf("tax = %d paid = %q, want %d %q", got.Tax, got.TaxPaidCompany, c.wantTax, c.wantPaid)
			}
		})
	}
}

func TestValidateAttachmentInput_TaxRange(t *testing.T) {
	for _, tax := range []int{-1, 3} {
		if _, err := validateAttachmentInput(component.Attachment{Sysid: 1, Tax: tax}); err == nil {
			t.Errorf("tax %d accepted", tax)
		}
	}
}

func TestResolvePut_HeldZeroStaysZero(t *testing.T) {
	// A row stored with tax 0 (UI before T033f) must not turn taxable on a GET -> PUT round trip.
	held := component.Attachment{Sysid: 80, Code: "T080", Name: "Baru", Sts: "F", Frz: 2, Source: component.SourceTemplate}
	c := component.CatalogEntry{Sysid: 80, Code: "T080", Name: "Baru", DefaultSts: "F", TaxClass: component.TaxClassTaxable}
	got, err := resolvePut(component.Attachment{Sysid: 80, Sts: "F", Frz: 2}, held, true, c, true)
	if err != nil || got.Tax != 0 || got.Source != component.SourceTemplate {
		t.Fatalf("got %+v err %v, want tax 0 and source TEMPLATE", got, err)
	}
}

func TestRuleEntry_LegacyTriggerTax(t *testing.T) {
	// NONE class on purpose: the legacy trigger hardcodes tax 2 / paid N regardless of the class.
	catalog := map[string]component.CatalogEntry{"T006": {Sysid: 6, Code: "T006", DefaultSts: "D", TaxClass: "NONE"}}
	a, err := ruleEntry(catalog, "T006")
	if err != nil || a.Tax != component.TaxTaxable || a.TaxPaidCompany != "N" {
		t.Fatalf("a = %+v err = %v, want tax 2 paid N", a, err)
	}
}

func TestNormalizeTemplate_TaxRange(t *testing.T) {
	for _, tax := range []int{-1, 3} {
		if _, err := normalizeTemplate([]component.TemplateItem{{Sysid: 1, Tax: tax}}); err == nil {
			t.Errorf("tax %d accepted", tax)
		}
	}
	if _, err := normalizeTemplate([]component.TemplateItem{{Sysid: 1, Tax: 0}, {Sysid: 2, Tax: 2}}); err != nil {
		t.Fatal(err)
	}
}
