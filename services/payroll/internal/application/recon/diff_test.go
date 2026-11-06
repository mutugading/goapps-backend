package recon

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

func dp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func key(t engine.CompType, id int) engine.CompKey { return engine.CompKey{Type: t, ID: id} }

func head(t, p, th string) *HeadTotals {
	return &HeadTotals{TotTunjangan: decimal.RequireFromString(t), TotPotongan: decimal.RequireFromString(p),
		TakeHome: decimal.RequireFromString(th)}
}

func TestCategorize(t *testing.T) {
	one, two, half := decimal.NewFromInt(1), decimal.NewFromInt(2), decimal.RequireFromString("-0.5")
	cases := []struct {
		name                  string
		inL, inT, att, master bool
		delta                 decimal.Decimal
		want                  string
	}{
		{"zero", true, true, false, false, decimal.Zero, CatMatch},
		{"zero one side (legacy drops zero lines)", false, true, false, false, decimal.Zero, CatMatch},
		{"only legacy", true, false, true, true, two, CatOnlyLegacy},
		{"only target", false, true, true, true, two, CatOnlyTarget},
		{"input att wins over master", true, true, true, true, two, CatInputAtt},
		{"input master", true, true, false, true, one, CatInputMaster},
		{"rounding 1", true, true, false, false, one.Neg(), CatRounding},
		{"rounding fraction", true, true, false, false, half, CatRounding},
		{"formula", true, true, false, false, two, CatFormula},
	}
	for _, c := range cases {
		if got := categorize(c.inL, c.inT, c.delta, c.att, c.master); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestMasterChanged(t *testing.T) {
	comps := []engine.EmpComponent{
		{Key: key(engine.CompT, 1), Amount: decimal.RequireFromString("3000000"), Sts: "F"},
		{Key: key(engine.CompP, 4), Amount: decimal.RequireFromString("10"), Sts: "F", Frozen: true},
	}
	cases := []struct {
		name string
		l    DiffLine
		want bool
	}{
		{"same", DiffLine{Key: key(engine.CompT, 1), Basis: dp("3000000.00"), Sts: "F"}, false},
		{"no basis info", DiffLine{Key: key(engine.CompT, 99)}, false},
		{"amount changed", DiffLine{Key: key(engine.CompT, 1), Basis: dp("3100000"), Sts: "F"}, true},
		{"sts changed", DiffLine{Key: key(engine.CompT, 1), Basis: dp("3000000"), Sts: "D"}, true},
		{"missing component", DiffLine{Key: key(engine.CompT, 2), Basis: dp("1"), Sts: "F"}, true},
		{"frozen component", DiffLine{Key: key(engine.CompP, 4), Basis: dp("10"), Sts: "F"}, true},
	}
	for _, c := range cases {
		if got := masterChanged([]DiffLine{c.l}, comps); got != c.want {
			t.Errorf("%s: %t, want %t", c.name, got, c.want)
		}
	}
}

// fixture: one NIK per category, plus a NIK without a recap (D2: lines but no head on either side).
func diffFixture() DiffInput {
	gapok := func(amt string) []engine.EmpComponent {
		return []engine.EmpComponent{{Key: key(engine.CompT, 1), Amount: decimal.RequireFromString(amt), Sts: "F"}}
	}
	l := func(id int, amt string) DiffLine {
		return DiffLine{Key: key(engine.CompT, id), Amount: dp(amt), Basis: dp("1000"), Sts: "F"}
	}
	tl := func(id int, amt string) DiffLine { return DiffLine{Key: key(engine.CompT, id), Amount: dp(amt)} }
	h := head("1000", "0", "1000")
	return DiffInput{
		Meta: DiffMeta{CompanyID: 1, Division: "SOLO", YYMM: 202608, Target: TargetDry, RuleSetID: 1},
		Legacy: map[string]DiffEmp{
			"A-MATCH":   {Lines: []DiffLine{l(1, "1000"), {Key: key(engine.CompP, 0), Amount: dp("0")}}, Head: h},
			"B-ROUND":   {Lines: []DiffLine{l(1, "1000")}, Head: h},
			"C-FORM":    {Lines: []DiffLine{l(1, "1000")}, Head: h},
			"D-ATT":     {Lines: []DiffLine{l(1, "1000")}, Head: h},
			"E-MASTER":  {Lines: []DiffLine{l(1, "1000")}, Head: h},
			"F-ONLYL":   {Lines: []DiffLine{l(1, "1000"), {Key: key(engine.CompP, 32), Amount: dp("50"), Basis: nil}}, Head: head("1000", "50", "900")},
			"H-NOREKAP": {Lines: []DiffLine{l(1, "1000")}},
		},
		Target: map[string]DiffEmp{
			"A-MATCH":   {Lines: []DiffLine{tl(1, "1000"), {Key: key(engine.CompT, 40), Amount: dp("0")}}, Head: h},
			"B-ROUND":   {Lines: []DiffLine{tl(1, "1001")}, Head: head("1001", "0", "1000")},
			"C-FORM":    {Lines: []DiffLine{tl(1, "1200")}, Head: head("1200", "0", "1200")},
			"D-ATT":     {Lines: []DiffLine{tl(1, "900")}, Head: head("900", "0", "900")},
			"E-MASTER":  {Lines: []DiffLine{tl(1, "1100")}, Head: head("1100", "0", "1100")},
			"F-ONLYL":   {Lines: []DiffLine{tl(1, "1000")}, Head: h},
			"G-ONLYT":   {Lines: []DiffLine{tl(1, "1000")}, Head: h},
			"H-NOREKAP": {Lines: []DiffLine{tl(1, "1000")}},
		},
		Components: map[string][]engine.EmpComponent{
			"A-MATCH": gapok("1000"), "B-ROUND": gapok("1000"), "C-FORM": gapok("1000"), "D-ATT": gapok("1000"),
			"E-MASTER": gapok("1100"), "F-ONLYL": gapok("1000"), "G-ONLYT": gapok("1000"), "H-NOREKAP": gapok("1000"),
		},
		AttChanged: map[string]bool{"D-ATT": true},
		Warnings:   []run.Issue{{NIK: "H-NOREKAP", Code: run.CodeMissingAttendance}},
	}
}

func TestDiff_EveryCategory(t *testing.T) {
	rep := Diff(diffFixture())
	want := map[string]int{CatMatch: 5, CatRounding: 1, CatFormula: 1, CatInputAtt: 1, CatInputMaster: 1,
		CatOnlyLegacy: 1, CatOnlyTarget: 1, CatAttDerived: 0, CatKnownDefect: 0}
	for c, n := range want {
		if got := rep.Summary.Categories[c]; got != n {
			t.Errorf("%s = %d, want %d", c, got, n)
		}
	}
	byNIK := map[string]LineDiff{}
	for _, l := range rep.Lines {
		byNIK[l.NIK] = l
	}
	for nik, c := range map[string]string{"B-ROUND": CatRounding, "C-FORM": CatFormula, "D-ATT": CatInputAtt,
		"E-MASTER": CatInputMaster, "F-ONLYL": CatOnlyLegacy, "G-ONLYT": CatOnlyTarget} {
		if byNIK[nik].Category != c {
			t.Errorf("%s: %+v, want %s", nik, byNIK[nik], c)
		}
	}
	if l := byNIK["F-ONLYL"]; l.Component != "P032" || l.Target != nil || !l.Delta.Equal(decimal.NewFromInt(-50)) {
		t.Errorf("only legacy line %+v", l)
	}
	if rep.Summary.PerComponent["T001"][CatFormula] != 1 || rep.Summary.PerComponent["P032"][CatOnlyLegacy] != 1 {
		t.Errorf("per component %+v", rep.Summary.PerComponent)
	}
	s := rep.Summary
	if s.NIKLegacy != 7 || s.NIKTarget != 8 {
		t.Errorf("nik legacy=%d target=%d", s.NIKLegacy, s.NIKTarget)
	}
	// heads: B..G differ (B tot_t differs though take-home equal); A and H (no head both sides) match.
	if s.HeadMismatch != 6 || len(rep.Heads) != 6 {
		t.Errorf("head mismatch %d %+v", s.HeadMismatch, rep.Heads)
	}
	// Σ take-home: legacy 5×1000 + 900 = 5900; target 1000+1000+1200+900+1100+1000+1000 = 7200.
	if !s.TakeHomeLegacy.Equal(decimal.NewFromInt(5900)) || !s.TakeHomeTarget.Equal(decimal.NewFromInt(7200)) ||
		!s.TakeHomeDelta.Equal(decimal.NewFromInt(1300)) || s.Parity {
		t.Errorf("summary %+v", s)
	}
	if len(rep.Warnings) != 1 {
		t.Errorf("warnings %+v", rep.Warnings)
	}
}

func TestDiff_Parity(t *testing.T) {
	in := diffFixture()
	for _, nik := range []string{"B-ROUND", "C-FORM", "D-ATT", "E-MASTER", "F-ONLYL", "G-ONLYT"} {
		delete(in.Legacy, nik)
		delete(in.Target, nik)
	}
	rep := Diff(in)
	if !rep.Summary.Parity || len(rep.Lines) != 0 || len(rep.Heads) != 0 || rep.Summary.Categories[CatMatch] != 4 {
		t.Fatalf("parity expected: %+v lines=%+v heads=%+v", rep.Summary, rep.Lines, rep.Heads)
	}
}

// Parity fails on ROUNDING alone, and on Σ take-home alone (INPUT_* lines do not block, the total does).
func TestDiff_ParityRules(t *testing.T) {
	base := diffFixture()
	keep := func(niks ...string) DiffInput {
		in := DiffInput{Meta: base.Meta, Legacy: map[string]DiffEmp{}, Target: map[string]DiffEmp{},
			Components: base.Components, AttChanged: base.AttChanged}
		for _, n := range niks {
			in.Legacy[n], in.Target[n] = base.Legacy[n], base.Target[n]
		}
		return in
	}
	if rep := Diff(keep("A-MATCH", "B-ROUND")); rep.Summary.Parity || !rep.Summary.TakeHomeDelta.IsZero() {
		t.Errorf("rounding must fail parity: %+v", rep.Summary)
	}
	if rep := Diff(keep("A-MATCH", "D-ATT")); rep.Summary.Parity || rep.Summary.Categories[CatFormula] != 0 {
		t.Errorf("take-home delta must fail parity: %+v", rep.Summary)
	}
}

func TestDiff_NullAmounts(t *testing.T) {
	in := DiffInput{
		Legacy: map[string]DiffEmp{"N": {Lines: []DiffLine{{Key: key(engine.CompT, 9)}, {Key: key(engine.CompT, 10)}}}},
		Target: map[string]DiffEmp{"N": {Lines: []DiffLine{{Key: key(engine.CompT, 9), Amount: dp("0")}, {Key: key(engine.CompT, 10), Amount: dp("3")}}}},
	}
	rep := Diff(in)
	if rep.Summary.Categories[CatMatch] != 1 || rep.Summary.Categories[CatFormula] != 1 {
		t.Fatalf("NULL counted as 0: %+v", rep.Summary.Categories)
	}
	if l := rep.Lines[0]; l.Legacy != nil || !l.Delta.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("line %+v", l)
	}
}

func TestTargetFromResults(t *testing.T) {
	rs := []engine.Result{
		{NIK: "X", Lines: []engine.Line{{Key: key(engine.CompT, 1), Amount: dp("100")}},
			Header: &engine.Header{TotTunjangan: decimal.NewFromInt(100), TakeHome: decimal.NewFromInt(100)}},
		{NIK: "Y", Lines: []engine.Line{{Key: key(engine.CompT, 1)}}},
	}
	m := TargetFromResults(rs)
	if m["X"].Head == nil || !m["X"].Head.TakeHome.Equal(decimal.NewFromInt(100)) || m["Y"].Head != nil ||
		m["Y"].Lines[0].Amount != nil {
		t.Fatalf("%+v", m)
	}
}

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]int64{"dry": 0, "run:12": 12} {
		if got, err := ParseTarget(in); err != nil || got != want {
			t.Errorf("%s: %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "run:", "run:0", "run:-1", "run:x", "legacy"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// D-49: ONLY_* rows of an included NIK fail parity even when every Δ is zero-sum; excluded NIKs do not.
func TestDiff_ParityOnlyRows(t *testing.T) {
	ln := func(id int, amt string) DiffLine { return DiffLine{Key: key(engine.CompT, id), Amount: dp(amt)} }
	for name, tc := range map[string]struct {
		legacy, target []DiffLine
		cat            string
	}{
		"only_target": {[]DiffLine{ln(1, "100")}, []DiffLine{ln(1, "100"), ln(2, "0.50")}, CatOnlyTarget},
		"only_legacy": {[]DiffLine{ln(1, "100"), ln(2, "0.50")}, []DiffLine{ln(1, "100")}, CatOnlyLegacy},
	} {
		in := DiffInput{
			Legacy: map[string]DiffEmp{"A": {Lines: tc.legacy}},
			Target: map[string]DiffEmp{"A": {Lines: tc.target}},
		}
		rep := Diff(in)
		if rep.Summary.Parity || rep.Summary.Categories[tc.cat] != 1 {
			t.Errorf("%s must fail parity: %+v", name, rep.Summary)
		}
		// Same row on an excluded NIK (synthetic MAIN, D-47) stays out of the count.
		in.Drift = DriftInput{SyntheticMain: map[string]bool{"A": true}}
		if rep := Diff(in); rep.Summary.Categories[tc.cat] != 0 {
			t.Errorf("%s: excluded NIK counted: %+v", name, rep.Summary.Categories)
		}
	}
	// Zero-Δ row on one side is MATCH and keeps parity.
	in := DiffInput{
		Legacy: map[string]DiffEmp{"A": {Lines: []DiffLine{ln(1, "100")}}},
		Target: map[string]DiffEmp{"A": {Lines: []DiffLine{ln(1, "100"), ln(2, "0")}}},
	}
	if rep := Diff(in); !rep.Summary.Parity {
		t.Errorf("zero-Δ only row must match: %+v", rep.Summary)
	}
}
