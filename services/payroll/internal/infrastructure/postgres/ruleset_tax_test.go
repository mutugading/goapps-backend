package postgres

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

func TestBuildTaxTables_LegacyRows(t *testing.T) {
	svs := []staticValue{
		{vs: vsEmpTax, code: "K1", name: "Kawin Dengan 1 Anak", f1: "B"},
		{vs: vsEmpTax, code: "X"}, // no category: left out (legacy join finds nothing)
		{vs: vsPayrollSetup, code: "B0.25", name: "6200001", short: "6500000", f1: "B", f2: "0.25"},
		{vs: vsPayrollSetup, code: "K1", name: "5250000", short: "5250000"},
		{vs: vsPayrollSetup, code: "BIAYAJABATAN", name: "5", short: "500000"},
		{vs: vsPayrollSetup, code: "TARIF15", name: "60000001", short: "250000000", f1: "15", f2: "2"},
		{vs: vsPayrollSetup, code: "SUBSIDIPPH", name: "10000000", short: "10000000"}, // not a tax table
		{vs: vsPayrollSetup, code: "BPJKESMAXJKT", name: "12000000", short: "12000000", f1: "BPJSKES", f2: "MAX"},
		{vs: vsPayrollSetup, code: "XFLAG", name: "1", short: "2", f1: "A", f2: "9"}, // field_01 A, not a TER code
	}
	tt, err := buildTaxTables(svs)
	if err != nil {
		t.Fatal(err)
	}
	if len(tt.Category) != 1 || tt.Category["K1"] != "B" {
		t.Fatalf("category: %v", tt.Category)
	}
	if b := tt.TER["B"]; len(b) != 1 || b[0].Min.String() != "6200001" || b[0].Max.String() != "6500000" || b[0].Rate.String() != "0.25" {
		t.Fatalf("TER B: %+v", b)
	}
	if len(tt.PTKPCompany) != 1 || tt.PTKPCompany["K1"].String() != "5250000" {
		t.Fatalf("PTKPCompany: %v", tt.PTKPCompany)
	}
	if len(tt.PTKP) != 1 || tt.PTKP["K1"].String() != "5250000" {
		t.Fatalf("PTKP: %v", tt.PTKP)
	}
	if tt.JobCost.Rate.String() != "5" || tt.JobCost.MaxMonthly.String() != "500000" {
		t.Fatalf("job cost: %+v", tt.JobCost)
	}
	if l := tt.Progressive; len(l) != 1 || l[0].Rate.String() != "15" || l[0].Max.String() != "250000000" {
		t.Fatalf("progressive: %+v", l)
	}
}

func TestBuildTaxTables_Duplicates_Error(t *testing.T) {
	cases := map[string][]staticValue{
		"job cost twice": {
			{vs: vsPayrollSetup, code: "BIAYAJABATAN", name: "5", short: "500000"},
			{vs: vsPayrollSetup, code: "BIAYAJABATAN", name: "5", short: "500000"},
		},
		"overlapping layers": {
			{vs: vsPayrollSetup, code: "TARIF5", name: "1", short: "60000000", f1: "5"},
			{vs: vsPayrollSetup, code: "TARIF5B", name: "50000000", short: "70000000", f1: "5"},
		},
	}
	for name, svs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := buildTaxTables(svs); !errors.Is(err, ErrStaticValueDup) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestBuildTaxTables_BadNumber_Error(t *testing.T) {
	_, err := buildTaxTables([]staticValue{{vs: vsPayrollSetup, code: "A0", name: "x", short: "1", f1: "A", f2: "0"}})
	if !errors.Is(err, ErrStaticValueParse) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckTaxTables_Empty_EngineErrors(t *testing.T) {
	ter, _ := buildTaxTables([]staticValue{
		{vs: vsEmpTax, code: "K0", f1: "A"},
		{vs: vsPayrollSetup, code: "A0", name: "0", short: "5400000", f1: "A", f2: "0"},
	})
	cases := []struct {
		name     string
		t        engine.TaxTables
		december bool
		want     error
	}{
		{"empty", engine.TaxTables{}, false, engine.ErrNoTaxTable},
		{"ter only, non-December", ter, false, nil},
		{"ter only, December", ter, true, engine.ErrNoAnnualTaxTable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := checkTaxTables(c.t, 1, c.december); !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestDecodeParams_StringsAndNumbers(t *testing.T) {
	p, err := decodeParams([]byte(`{"days":"DAYS_MONTH","mult":1.5,"on":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if p["days"] != "DAYS_MONTH" || p["mult"] != "1.5" || p["on"] != "true" {
		t.Fatalf("got %v", p)
	}
}

func TestBuildTaxTables_PTKPCompanyExcludesTK123(t *testing.T) {
	var svs []staticValue
	for _, c := range []string{"TK", "K", "K0", "K1", "K2", "K3", "TK1", "TK2", "TK3"} {
		svs = append(svs, staticValue{vs: vsPayrollSetup, code: c, name: "4500000", short: "4500000"})
	}
	tt, err := buildTaxTables(svs)
	if err != nil {
		t.Fatal(err)
	}
	if len(tt.PTKPCompany) != 6 || len(tt.PTKP) != 9 {
		t.Fatalf("PTKPCompany %d (want 6), PTKP %d (want 9)", len(tt.PTKPCompany), len(tt.PTKP))
	}
	for _, c := range []string{"TK1", "TK2", "TK3"} {
		if _, ok := tt.PTKPCompany[c]; ok {
			t.Errorf("%s must not be in PTKPCompany", c)
		}
	}
}

func TestMatchCodeFormula(t *testing.T) {
	t21 := engine.CompKey{Type: engine.CompT, ID: 21}
	cases := []struct {
		name string
		k    engine.CompKey
		c    ComponentRule
		ok   bool
	}{
		{"restates code", t21, ComponentRule{Formula: engine.FormulaOTHours, Params: engine.Params{"col": "lembur_1", "mult": "1.5"}}, true},
		{"numeric params compare as decimal", t21, ComponentRule{Formula: engine.FormulaOTHours, Params: engine.Params{"col": "lembur_1", "mult": "1.50"}}, true},
		{"numeric params differ", t21, ComponentRule{Formula: engine.FormulaOTHours, Params: engine.Params{"col": "lembur_1", "mult": "1.51"}}, false},
		{"text params differ", t21, ComponentRule{Formula: engine.FormulaOTHours, Params: engine.Params{"col": "lembur_2", "mult": "1.5"}}, false},
		{"other params", t21, ComponentRule{Formula: engine.FormulaOTHours, Params: engine.Params{"col": "lembur_1", "mult": "2"}}, false},
		{"other formula", t21, ComponentRule{Formula: engine.FormulaAddition, Params: engine.Params{}}, false},
		{"remark", engine.CompKey{Type: engine.CompT, ID: 1}, ComponentRule{Formula: engine.FormulaFixedOrProrate, Params: engine.Params{}, RemarkTpl: "x"}, false},
		{"unmapped id", engine.CompKey{Type: engine.CompT, ID: 80}, ComponentRule{Formula: engine.FormulaLegacyNull, Params: engine.Params{}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := matchCodeFormula(c.k, c.c)
			if (err == nil) != c.ok || (err != nil && !IsRuleError(err)) {
				t.Fatalf("err = %v, want ok=%v and a rule error", err, c.ok)
			}
		})
	}
}

func TestCheckRequiredConstants(t *testing.T) {
	ok := map[string]decimal.Decimal{engine.ConstKesMax: decimal.NewFromInt(12000000), engine.ConstPenMax: decimal.NewFromInt(11086300)}
	if err := checkRequiredConstants(ok); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{engine.ConstKesMax, engine.ConstPenMax} {
		m := map[string]decimal.Decimal{engine.ConstKesMax: decimal.NewFromInt(1), engine.ConstPenMax: decimal.NewFromInt(1)}
		delete(m, k)
		if err := checkRequiredConstants(m); !errors.Is(err, ErrRuleConstMissing) || !IsRuleError(err) {
			t.Errorf("without %s: err = %v", k, err)
		}
		m[k] = decimal.Zero
		if err := checkRequiredConstants(m); !errors.Is(err, ErrRuleConstMissing) {
			t.Errorf("%s = 0: err = %v", k, err)
		}
	}
}
