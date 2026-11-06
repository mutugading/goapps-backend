package recon

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// driftFixture: A matches; S has a synthetic MAIN (target skips the header, adds lines); I is paid by legacy but
// inactive in the master now (target has nothing); F has T015 frozen now but paid by legacy; R is a Rp 1
// difference of an included NIK so parity still sees it.
func driftFixture() DiffInput {
	ln := func(t engine.CompType, id int, amt string) DiffLine {
		return DiffLine{Key: key(t, id), Amount: dp(amt)}
	}
	return DiffInput{
		Legacy: map[string]DiffEmp{
			"A": {Lines: []DiffLine{ln(engine.CompT, 1, "1000")}, Head: head("1000", "0", "1000")},
			"S": {Lines: []DiffLine{ln(engine.CompT, 40, "25")}, Head: head("25", "0", "25")},
			"I": {Lines: []DiffLine{ln(engine.CompT, 1, "1500"), ln(engine.CompP, 32, "15")}, Head: head("1500", "15", "1485")},
			"F": {Lines: []DiffLine{ln(engine.CompT, 1, "1000"), ln(engine.CompT, 15, "45")}, Head: head("1045", "0", "1045")},
		},
		Target: map[string]DiffEmp{
			"A": {Lines: []DiffLine{ln(engine.CompT, 1, "1000")}, Head: head("1000", "0", "1000")},
			"S": {Lines: []DiffLine{ln(engine.CompT, 40, "25"), ln(engine.CompT, 2, "10")}},
			"F": {Lines: []DiffLine{ln(engine.CompT, 1, "1000")}, Head: head("1000", "0", "1000")},
		},
		Components: map[string][]engine.EmpComponent{
			"A": {{Key: key(engine.CompT, 1), Amount: decimal.NewFromInt(1000)}},
			"F": {{Key: key(engine.CompT, 1), Amount: decimal.NewFromInt(1000)}},
		},
		Drift: DriftInput{
			SyntheticMain: map[string]bool{"S": true},
			Inactive:      map[string]bool{"I": true, "S": true},
			Frozen:        map[string]map[engine.CompKey]bool{"F": {key(engine.CompT, 15): true}},
		},
	}
}

func TestDiff_ExclusionsGiveParity(t *testing.T) {
	rep := Diff(driftFixture())
	s := rep.Summary
	if !s.Parity || s.HeadMismatch != 0 || !s.TakeHomeDelta.IsZero() || !s.TakeHomeLegacy.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("included part must be at parity: %+v", s)
	}
	syn, drift := s.Excluded[ExclSyntheticMain], s.Excluded[ExclMasterDrift]
	if syn.NIK != 1 || syn.Lines != 1 || syn.HeadMismatch != 1 || !syn.TakeHomeDelta.Equal(decimal.NewFromInt(-25)) {
		t.Errorf("synthetic %+v", syn)
	}
	// I: −1485 (all lines only legacy), F: T015 −45.
	if drift.NIK != 2 || drift.Lines != 3 || drift.OtherLines != 0 || drift.HeadMismatch != 2 || !drift.TakeHomeDelta.Equal(decimal.NewFromInt(-1530)) {
		t.Errorf("drift %+v", drift)
	}
	want := map[string]int{CatMatch: 3, CatKnownDefect: 1, CatMasterDrift: 3, CatFormula: 0, CatRounding: 0,
		CatOnlyLegacy: 0, CatOnlyTarget: 0, CatInputMaster: 0}
	for c, n := range want {
		if s.Categories[c] != n {
			t.Errorf("%s = %d, want %d", c, s.Categories[c], n)
		}
	}
	if s.NIKLegacy != 4 || s.NIKTarget != 3 {
		t.Errorf("nik legacy=%d target=%d", s.NIKLegacy, s.NIKTarget)
	}
	codes := map[string]string{}
	for _, w := range rep.Warnings {
		codes[w.NIK] = w.Code
	}
	if codes["S"] != CodeExclSyntheticMain || codes["I"] != CodeExclMasterDrift || codes["F"] != CodeExclMasterDrift || len(codes) != 3 {
		t.Errorf("warnings %+v", rep.Warnings)
	}
	for _, h := range rep.Heads {
		if h.Excluded == "" {
			t.Errorf("head %s listed without reason", h.NIK)
		}
	}
}

func TestDiff_ExclusionsDoNotHideIncludedDiffs(t *testing.T) {
	in := driftFixture()
	in.Target["A"] = DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 1), Amount: dp("1001")}}, Head: head("1001", "0", "1001")}
	rep := Diff(in)
	if s := rep.Summary; s.Parity || s.Categories[CatRounding] != 1 || s.HeadMismatch != 1 || !s.TakeHomeDelta.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("included Rp 1 must fail parity: %+v", s)
	}
}

func TestExclusionOf(t *testing.T) {
	paid := DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 15), Amount: dp("45")}}, Head: head("45", "0", "45")}
	frozen := map[string]map[engine.CompKey]bool{"N": {key(engine.CompT, 15): true}}
	cases := []struct {
		name  string
		d     DriftInput
		leg   DiffEmp
		comps []engine.EmpComponent
		want  string
	}{
		{"none", DriftInput{}, paid, nil, ""},
		{"synthetic wins", DriftInput{SyntheticMain: map[string]bool{"N": true}, Inactive: map[string]bool{"N": true}}, paid, nil, ExclSyntheticMain},
		{"inactive with MAIN", DriftInput{Inactive: map[string]bool{"N": true}}, paid, nil, ExclMasterDrift},
		{"inactive without MAIN", DriftInput{Inactive: map[string]bool{"N": true}}, DiffEmp{Lines: paid.Lines}, nil, ""},
		{"late join no legacy (D-50)", DriftInput{LateJoin: map[string]bool{"N": true}}, DiffEmp{}, nil, ExclNoLegacyD2},
		{"late join but legacy has lines", DriftInput{LateJoin: map[string]bool{"N": true}}, DiffEmp{Lines: paid.Lines}, nil, ""},
		{"late join but legacy has head", DriftInput{LateJoin: map[string]bool{"N": true}}, DiffEmp{Head: paid.Head}, nil, ""},
		{"no late join flag, no legacy", DriftInput{}, DiffEmp{}, nil, ""},
		{"missing from master", DriftInput{Missing: map[string]bool{"N": true}}, paid, nil, ExclMasterDrift},
		{"frozen paid", DriftInput{Frozen: frozen}, paid, nil, ExclMasterDrift},
		{"frozen but zero in legacy", DriftInput{Frozen: frozen},
			DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 15), Amount: dp("0")}}}, nil, ""},
		{"frozen row plus active row", DriftInput{Frozen: frozen}, paid,
			[]engine.EmpComponent{{Key: key(engine.CompT, 15), Amount: decimal.NewFromInt(45)}}, ""},
		{"frozen P4 (D-28d) only loaded frozen", DriftInput{Frozen: frozen}, paid,
			[]engine.EmpComponent{{Key: key(engine.CompT, 15), Frozen: true}}, ExclMasterDrift},
	}
	for _, c := range cases {
		if got := exclusionOf(c.d, "N", c.leg, c.comps).reason; got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A frozen-component drift still shows Δ on other components separately (review T019h MED1).
func TestDiff_DriftOtherLines(t *testing.T) {
	in := driftFixture()
	in.Target["F"] = DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 1), Amount: dp("1200")}}, Head: head("1200", "0", "1200")}
	rep := Diff(in)
	if x := rep.Summary.Excluded[ExclMasterDrift]; x.OtherLines != 1 || x.Lines != 4 {
		t.Fatalf("other lines %+v", x)
	}
	if !rep.Summary.Parity {
		t.Fatalf("excluded NIK must not fail parity: %+v", rep.Summary)
	}
}

// D-50: a late-joining NIK without any legacy row is excluded, reported as an aggregate and keeps parity.
func TestDiff_NoLegacyD2(t *testing.T) {
	in := driftFixture()
	in.Target["L"] = DiffEmp{Lines: []DiffLine{{Key: key(engine.CompT, 11), Amount: dp("100")}, {Key: key(engine.CompP, 11), Amount: dp("20")}}}
	in.Drift.LateJoin = map[string]bool{"L": true}
	rep := Diff(in)
	x := rep.Summary.Excluded[ExclNoLegacyD2]
	if x.NIK != 1 || x.Lines != 2 || x.HeadMismatch != 0 {
		t.Fatalf("no-legacy %+v", x)
	}
	if !rep.Summary.Parity || rep.Summary.Categories[CatOnlyTarget] != 0 {
		t.Fatalf("parity must hold: %+v", rep.Summary)
	}
	found := false
	for _, w := range rep.Warnings {
		found = found || (w.NIK == "L" && w.Code == CodeExclNoLegacyD2)
	}
	if !found {
		t.Error("missing EXCLUDED_NO_LEGACY_D2 warning")
	}
}
