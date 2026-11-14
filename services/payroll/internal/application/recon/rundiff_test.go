package recon

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

func TestParseAgainst(t *testing.T) {
	for in, want := range map[string]int64{"active": 0, "legacy": 0, "run:12": 12} {
		if got, err := ParseAgainst(in); err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "dry", "run:", "run:0", "run:-1", "run:x", "ACTIVE"} {
		if _, err := ParseAgainst(in); !errors.Is(err, ErrBadAgainst) {
			t.Errorf("%q: err = %v, want ErrBadAgainst", in, err)
		}
	}
}

// Two stored runs through the shared Diff: same input = MATCH only (parity); a changed basis = INPUT_MASTER,
// a changed amount with the same basis = FORMULA, a line on one side = ONLY_*.
func TestRunPairCategories(t *testing.T) {
	side := func(amt, basis string, sts string) DiffEmp {
		return DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 1), Amount: dp(amt), Basis: dp(basis), Sts: sts}},
			Head: head(amt, "0", amt)}
	}
	pair := func(self, other map[string]DiffEmp) DiffInput {
		in := DiffInput{Legacy: other, Target: map[string]DiffEmp{}, Components: map[string][]engine.EmpComponent{}}
		for nik, e := range self {
			in.Components[nik] = componentsOfLines(e.Lines)
			in.Target[nik] = DiffEmp{Lines: stripInputs(e.Lines), Head: e.Head}
		}
		return in
	}
	same := map[string]DiffEmp{"A": side("100", "100", "F"), "B": side("200", "200", "F")}
	rep := Diff(pair(same, same))
	if !rep.Summary.Parity || len(rep.Lines) != 0 || rep.Summary.Categories[CatMatch] != 2 || rep.Summary.HeadMismatch != 0 {
		t.Fatalf("self diff = %+v lines %v", rep.Summary, rep.Lines)
	}
	self := map[string]DiffEmp{"A": side("150", "150", "F"), "B": side("250", "200", "F"), "C": side("10", "10", "F")}
	rep = Diff(pair(self, same))
	got := map[string]string{}
	for _, l := range rep.Lines {
		got[l.NIK] = l.Category
	}
	if got["A"] != CatInputMaster || got["B"] != CatFormula || got["C"] != CatOnlyTarget || rep.Summary.Parity {
		t.Fatalf("categories = %v parity %t", got, rep.Summary.Parity)
	}
	if rep.Summary.TakeHomeDelta.String() != "110" {
		t.Fatalf("take-home delta = %s", rep.Summary.TakeHomeDelta)
	}
}

func TestComponentsOfLinesSkipsLinesWithoutInputs(t *testing.T) {
	cs := componentsOfLines([]DiffLine{{Key: key(engine.CompT, 1), Amount: dp("1")}, {Key: key(engine.CompP, 2), Sts: "D"}})
	if len(cs) != 1 || cs[0].Key != key(engine.CompP, 2) || cs[0].Sts != "D" || !cs[0].Amount.IsZero() {
		t.Fatalf("components = %+v", cs)
	}
}
