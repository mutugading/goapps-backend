package recon

import (
	"testing"

	"github.com/shopspring/decimal"
)

// ATT_DERIVED (05§5): a (NIK, field) pair counts when derived ≠ MPAR, NULL = 0; it never touches line categories
// or the v1 parity verdict, and an excluded NIK (D-47) is listed but not counted.
func TestDiff_AttDerived(t *testing.T) {
	in := diffFixture()
	for _, nik := range []string{"B-ROUND", "C-FORM", "D-ATT", "E-MASTER", "F-ONLYL", "G-ONLYT"} {
		delete(in.Legacy, nik)
		delete(in.Target, nik)
	}
	in.Derived = []AttDerivedRow{
		{NIK: "A-MATCH", Field: DerivedShift, MPAR: dp("5"), Derived: dp("5.00")}, // equal
		{NIK: "A-MATCH", Field: DerivedOtiw, MPAR: dp("2"), Derived: dp("3")},     // +1
		{NIK: "H-NOREKAP", Field: DerivedShift, MPAR: nil, Derived: dp("0")},      // NULL = 0
		{NIK: "H-NOREKAP", Field: DerivedOtiw, MPAR: dp("1"), Derived: nil},       // −1
		{NIK: "Z-SYNTH", Field: DerivedShift, MPAR: dp("4"), Derived: dp("0")},    // excluded below
	}
	in.Drift.SyntheticMain = map[string]bool{"Z-SYNTH": true}
	in.Legacy["Z-SYNTH"] = DiffEmp{Head: head("0", "0", "0")}
	rep := Diff(in)
	if got := rep.Summary.Categories[CatAttDerived]; got != 2 {
		t.Fatalf("ATT_DERIVED = %d, want 2: %+v", got, rep.AttDerived)
	}
	if len(rep.AttDerived) != 3 || len(rep.Lines) != 0 || !rep.Summary.Parity {
		t.Fatalf("derived %+v lines %+v summary %+v", rep.AttDerived, rep.Lines, rep.Summary)
	}
	want := []struct {
		nik, field, delta, excl string
	}{{"A-MATCH", DerivedOtiw, "1", ""}, {"H-NOREKAP", DerivedOtiw, "-1", ""}, {"Z-SYNTH", DerivedShift, "-4", ExclSyntheticMain}}
	for i, w := range want {
		d := rep.AttDerived[i]
		if d.NIK != w.nik || d.Field != w.field || !d.Delta.Equal(decimal.RequireFromString(w.delta)) || d.Excluded != w.excl {
			t.Errorf("pos %d: %+v, want %+v", i, d, w)
		}
	}
}
