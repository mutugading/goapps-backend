package engine

import (
	"errors"
	"testing"
)

var keyP0 = CompKey{CompP, 0}

// subT is a taxable T component in the subsidy basis (tax_subsidy = Y).
func subT(id int, amt string) EmpComponent {
	c := taxT(id, amt, "F")
	c.TaxSubsidy = true
	return c
}

// ruleV1 is the migrated MGT_PAYROLL_SUBSIDI_PPH row: SOLO, basis 202601, limit 10 jt, FULL_PPH.
func ruleV1() SubsidyRule {
	return SubsidyRule{ID: 7, Name: "SOLO 2026", Priority: 100, ValidFrom: 202601, Divisions: []string{"SOLO"},
		BasisPeriod: 202601, Limit: d("10000000"), Kind: SubsidyFullPPh}
}

// subInput: T1 5.600.000 (P1 TER A 0,25 % = 14.000) with a P0 row.
func subInput(basis string, rules ...SubsidyRule) Input {
	in := pphInput("TK", subT(1, "5600000"), pComp(0, "0", "F", false))
	in.Subsidy.Rules = rules
	if basis != "" {
		b := d(basis)
		in.Subsidy.Basis = &b
	}
	return in
}

// Golden angka tangan (06§6.4, 12§4): P0 = −P1 bila rule cocok dan basis ≤ batas, selain itu 0.
func TestPPhSubsidy(t *testing.T) {
	cases := []struct {
		name     string
		in       func() Input
		want     string
		ruleMeta string
	}{
		{"aktif, basis 202601 ≤ batas", func() Input { return subInput("9000000", ruleV1()) }, "-14000", "7"},
		{"basis tepat batas", func() Input { return subInput("10000000", ruleV1()) }, "-14000", "7"},
		{"di atas batas", func() Input { return subInput("10000001", ruleV1()) }, "0", "7"},
		{"tanpa rule (YN N / tanpa baris)", func() Input { return subInput("9000000") }, "0", ""},
		{"divisi lain", func() Input {
			in := subInput("9000000", ruleV1())
			in.Division = "JAKARTA"
			return in
		}, "0", ""},
		{"rule belum berlaku", func() Input {
			r := ruleV1()
			r.ValidFrom = 202611
			return subInput("9000000", r)
		}, "0", ""},
		// basis 202601 nil/0 → baris berjalan tax_subsidy = Y (5.600.000, T7 bukan basis).
		{"fallback komponen berjalan", func() Input {
			in := subInput("", ruleV1())
			in.Components = append(in.Components, taxT(7, "0", "F"))
			return in
		}, "-14000", "7"},
		{"fallback berjalan di atas batas", func() Input {
			in := subInput("0", ruleV1())
			in.Components = append(in.Components, subT(7, "4500000")) // basis 10.100.000
			return in
		}, "0", "7"},
		{"prioritas: rule pertama yang cocok", func() Input {
			low := ruleV1()
			low.ID, low.Priority, low.Limit = 9, 10, d("5000000")
			return subInput("9000000", ruleV1(), low)
		}, "0", "9"},
		{"override INCLUDE di atas batas", func() Input {
			in := subInput("20000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideInclude, Reason: "kebijakan direksi"}
			return in
		}, "-14000", ""},
		{"override EXCLUDE", func() Input {
			in := subInput("9000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideExclude, Reason: "PPh manual"}
			return in
		}, "0", ""},
		{"override AMOUNT", func() Input {
			in := subInput("9000000", ruleV1())
			in.Subsidy.Override = &SubsidyOverride{Action: OverrideAmount, Amount: d("5000"), Reason: "sebagian"}
			return in
		}, "-5000", ""},
		// P1 NULL (bracket hilang) → −NULL tetap NULL, seperti V_PPH × −1 legacy.
		{"P1 NULL", func() Input {
			in := subInput("9000000", ruleV1())
			in.Components[0] = subT(1, "7000000")
			return in
		}, "", "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := lineOf(t, c.in(), keyP0)
			if got := amountStr(l); got != c.want {
				t.Errorf("P0 = %q, want %q (remark %q)", got, c.want, l.Remark)
			}
			if got := l.Meta[MetaSubsidyRule]; got != c.ruleMeta {
				t.Errorf("meta rule = %q, want %q", got, c.ruleMeta)
			}
		})
	}
	t.Run("meta mencatat basis, batas, override", func(t *testing.T) {
		l := lineOf(t, subInput("9000000", ruleV1()), keyP0)
		if l.Meta[MetaSubsidyBasis] != "9000000" || l.Meta[MetaSubsidyLimit] != "10000000" {
			t.Errorf("meta = %v", l.Meta)
		}
		in := subInput("9000000", ruleV1())
		in.Subsidy.Override = &SubsidyOverride{Action: OverrideExclude, Reason: "x"}
		if l = lineOf(t, in, keyP0); l.Meta[MetaSubsidyOverride] != OverrideExclude {
			t.Errorf("meta = %v", l.Meta)
		}
		if l = lineOf(t, in, CompKey{CompP, 1}); l.Meta != nil {
			t.Errorf("P1 meta = %v, want nil", l.Meta)
		}
	})
	t.Run("take-home tanpa potongan PPh", func(t *testing.T) {
		res, err := Calculate(subInput("9000000", ruleV1()))
		if err != nil {
			t.Fatal(err)
		}
		if res.Header == nil || res.Header.TotPotongan.String() != "0" {
			t.Errorf("header = %+v", res.Header)
		}
	})
	t.Run("jenis tidak dikenal", func(t *testing.T) {
		r := ruleV1()
		r.Kind = "EXPR"
		if _, err := Calculate(subInput("9000000", r)); !errors.Is(err, ErrSubsidyKind) {
			t.Fatalf("err = %v", err)
		}
		in := subInput("9000000", ruleV1())
		in.Subsidy.Override = &SubsidyOverride{Action: "X"}
		if _, err := Calculate(in); !errors.Is(err, ErrSubsidyKind) {
			t.Fatalf("override err = %v", err)
		}
	})
}
