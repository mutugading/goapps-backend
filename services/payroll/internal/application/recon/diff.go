package recon

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Diff categories of 09§4 step 3. ATT_DERIVED waits for the attendance sync (D-41). KNOWN_DEFECT is the v2
// category of 09§4; under v1 it only holds the lines of NIKs with a synthetic MAIN (defect D2, D-47).
// MASTER_DRIFT (D-47) holds the lines of NIKs whose current master no longer matches the legacy period.
const (
	CatMatch       = "MATCH"
	CatRounding    = "ROUNDING"
	CatFormula     = "FORMULA"
	CatInputAtt    = "INPUT_ATT"
	CatAttDerived  = "ATT_DERIVED"
	CatInputMaster = "INPUT_MASTER"
	CatOnlyLegacy  = "ONLY_LEGACY"
	CatOnlyTarget  = "ONLY_TARGET"
	CatKnownDefect = "KNOWN_DEFECT"
	CatMasterDrift = "MASTER_DRIFT"
)

// Categories lists every category in report order.
var Categories = []string{CatMatch, CatRounding, CatFormula, CatInputAtt, CatAttDerived, CatInputMaster,
	CatOnlyLegacy, CatOnlyTarget, CatMasterDrift, CatKnownDefect}

// roundingLimit is the largest |Δ| (rupiah) that counts as ROUNDING (09§4).
var roundingLimit = decimal.NewFromInt(1)

// DiffLine is one DETAIL row. Amount nil = NULL (D10). Basis and Sts are the legacy PD_BASIS_AMOUNT
// (= MPCE_AMOUNT) and PD_STATUS (= MPCE_STS) the line was calculated from; only the legacy side uses them.
type DiffLine struct {
	Key    engine.CompKey
	Amount *decimal.Decimal
	Basis  *decimal.Decimal
	Sts    string
}

// HeadTotals are the header figures of the parity criteria.
type HeadTotals struct {
	TotTunjangan decimal.Decimal `json:"tot_tunjangan"`
	TotPotongan  decimal.Decimal `json:"tot_potongan"`
	TakeHome     decimal.Decimal `json:"take_home"`
}

// DiffEmp is the result of one NIK on one side. Head nil = no MAIN row (D2).
type DiffEmp struct {
	Lines []DiffLine
	Head  *HeadTotals
}

// DiffMeta identifies a recon run in the report.
type DiffMeta struct {
	CompanyID        int16  `json:"company_id"`
	Division         string `json:"division"`
	YYMM             int    `json:"yymm"`
	Target           string `json:"target"` // dry | run:<id>
	RuleSetID        int    `json:"rule_set_id"`
	LegacyRunID      int64  `json:"legacy_run_id"`
	LegacySnapshotID int64  `json:"legacy_snapshot_id"`
	TargetSnapshotID int64  `json:"target_snapshot_id"`
}

// DiffInput is everything Diff compares; DiffSource builds it read-only (D-41).
type DiffInput struct {
	Meta   DiffMeta
	Legacy map[string]DiffEmp
	Target map[string]DiffEmp
	// Components is the employee master the target was calculated from (frozen rows included, D-28d).
	Components map[string][]engine.EmpComponent
	// AttChanged marks NIKs whose recap in the target snapshot differs from the legacy snapshot.
	AttChanged map[string]bool
	// Drift holds the master attributes of the D-47 exclusions; zero value = no exclusion.
	Drift    DriftInput
	Warnings []run.Issue
}

// LineDiff is a non-MATCH line. Delta = target − legacy, NULL counted as 0.
type LineDiff struct {
	NIK       string           `json:"nik"`
	Component string           `json:"component"` // T001 / P032
	Legacy    *decimal.Decimal `json:"legacy"`
	Target    *decimal.Decimal `json:"target"`
	Delta     decimal.Decimal  `json:"delta"`
	Category  string           `json:"category"`
}

// HeadDiff is a NIK whose header totals or take-home differ (or exist on one side only).
type HeadDiff struct {
	NIK      string      `json:"nik"`
	Legacy   *HeadTotals `json:"legacy"`
	Target   *HeadTotals `json:"target"`
	Excluded string      `json:"excluded,omitempty"` // D-47 reason; not counted in HeadMismatch
}

// DiffSummary holds the counts and the parity verdict of rule set v1 (09§4). Counts, head mismatches and the
// take-home sums cover the included NIKs only; excluded NIKs (D-47) are aggregated in Excluded.
type DiffSummary struct {
	Categories     map[string]int            `json:"categories"`
	PerComponent   map[string]map[string]int `json:"per_component"` // component → category → count (non-MATCH)
	NIKLegacy      int                       `json:"nik_legacy"`
	NIKTarget      int                       `json:"nik_target"`
	HeadMismatch   int                       `json:"head_mismatch"`
	TakeHomeLegacy decimal.Decimal           `json:"take_home_legacy"`
	TakeHomeTarget decimal.Decimal           `json:"take_home_target"`
	TakeHomeDelta  decimal.Decimal           `json:"take_home_delta"`
	Parity         bool                      `json:"parity"`
	Excluded       map[string]ExclusionStat  `json:"excluded"` // reason → aggregate (D-47)
}

// DiffReport is the JSON report for CI. It holds real NIKs and amounts: never commit it.
type DiffReport struct {
	DiffMeta
	Summary  DiffSummary `json:"summary"`
	Lines    []LineDiff  `json:"lines"`
	Heads    []HeadDiff  `json:"heads"`
	Warnings []run.Issue `json:"warnings"`
}

// Diff joins legacy and target per (nik, type, id) and categorizes every line (09§4 step 3).
// Precedence for a non-zero Δ: ONLY_* (row on one side) → INPUT_ATT → INPUT_MASTER → ROUNDING (|Δ| ≤ 1) → FORMULA.
// A row missing on one side with Δ = 0 is MATCH: legacy DETAIL keeps only non-zero lines plus P0.
func Diff(in DiffInput) DiffReport {
	rep := DiffReport{DiffMeta: in.Meta, Lines: []LineDiff{}, Heads: []HeadDiff{}, Warnings: in.Warnings}
	if rep.Warnings == nil {
		rep.Warnings = []run.Issue{}
	}
	s := &rep.Summary
	s.Categories = make(map[string]int, len(Categories))
	for _, c := range Categories {
		s.Categories[c] = 0
	}
	s.PerComponent = map[string]map[string]int{}
	s.Excluded = make(map[string]ExclusionStat, len(Exclusions))
	for _, e := range Exclusions {
		s.Excluded[e] = ExclusionStat{}
	}

	for _, nik := range unionNIKs(in.Legacy, in.Target) {
		leg, tgt := in.Legacy[nik], in.Target[nik]
		if ex := exclusionOf(in.Drift, nik, leg, in.Components[nik]); ex.reason != "" {
			rep.addExcluded(nik, ex, leg, tgt)
			continue
		}
		rep.addNIK(nik, leg, tgt, in.AttChanged[nik], masterChanged(leg.Lines, in.Components[nik]))
	}
	s.TakeHomeDelta = s.TakeHomeTarget.Sub(s.TakeHomeLegacy)
	// Header per NIK (09§4) is part of parity: a mismatch of an included NIK fails it.
	s.Parity = s.Categories[CatFormula] == 0 && s.Categories[CatRounding] == 0 && s.TakeHomeDelta.IsZero() &&
		s.HeadMismatch == 0
	return rep
}

// addNIK joins the lines of one NIK and adds them and its header to the report.
func (rep *DiffReport) addNIK(nik string, leg, tgt DiffEmp, att, master bool) {
	s := &rep.Summary
	countNIK(s, leg, tgt)
	lm, tm := indexLines(leg.Lines), indexLines(tgt.Lines)
	for _, k := range unionKeys(lm, tm) {
		l, inL := lm[k]
		t, inT := tm[k]
		delta := orZero(t.Amount).Sub(orZero(l.Amount))
		cat := categorize(inL, inT, delta, att, master)
		if cat == CatMatch {
			s.Categories[CatMatch]++
			continue
		}
		rep.addLine(LineDiff{NIK: nik, Component: k.String(), Legacy: l.Amount, Target: t.Amount, Delta: delta, Category: cat})
	}
	if leg.Head != nil {
		s.TakeHomeLegacy = s.TakeHomeLegacy.Add(leg.Head.TakeHome)
	}
	if tgt.Head != nil {
		s.TakeHomeTarget = s.TakeHomeTarget.Add(tgt.Head.TakeHome)
	}
	if !sameHead(leg.Head, tgt.Head) {
		s.HeadMismatch++
		rep.Heads = append(rep.Heads, HeadDiff{NIK: nik, Legacy: leg.Head, Target: tgt.Head})
	}
}

func countNIK(s *DiffSummary, leg, tgt DiffEmp) {
	if len(leg.Lines) > 0 || leg.Head != nil {
		s.NIKLegacy++
	}
	if len(tgt.Lines) > 0 || tgt.Head != nil {
		s.NIKTarget++
	}
}

// addLine records a non-MATCH line and its category count.
func (rep *DiffReport) addLine(l LineDiff) {
	s := &rep.Summary
	s.Categories[l.Category]++
	if s.PerComponent[l.Component] == nil {
		s.PerComponent[l.Component] = map[string]int{}
	}
	s.PerComponent[l.Component][l.Category]++
	rep.Lines = append(rep.Lines, l)
}

func categorize(inLegacy, inTarget bool, delta decimal.Decimal, attChanged, masterChanged bool) string {
	switch {
	case delta.IsZero():
		return CatMatch
	case !inTarget:
		return CatOnlyLegacy
	case !inLegacy:
		return CatOnlyTarget
	case attChanged:
		return CatInputAtt
	case masterChanged:
		return CatInputMaster
	case delta.Abs().LessThanOrEqual(roundingLimit):
		return CatRounding
	default:
		return CatFormula
	}
}

// masterChanged reports whether the employee master differs from what legacy calculated with:
// a legacy line whose component is missing (or frozen) in the target input, or whose PD_BASIS_AMOUNT /
// PD_STATUS differs from the component amount / status. Additions are not visible in DETAIL.
func masterChanged(legacy []DiffLine, comps []engine.EmpComponent) bool {
	idx := make(map[engine.CompKey]engine.EmpComponent, len(comps))
	for _, c := range comps {
		if !c.Frozen {
			idx[c.Key] = c
		}
	}
	for _, l := range legacy {
		if l.Basis == nil && l.Sts == "" {
			continue
		}
		c, ok := idx[l.Key]
		if !ok {
			return true
		}
		if l.Basis != nil && !c.Amount.Equal(*l.Basis) {
			return true
		}
		if l.Sts != "" && c.Sts != l.Sts {
			return true
		}
	}
	return false
}

func sameHead(a, b *HeadTotals) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.TotTunjangan.Equal(b.TotTunjangan) && a.TotPotongan.Equal(b.TotPotongan) && a.TakeHome.Equal(b.TakeHome)
}

func orZero(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}

func indexLines(ls []DiffLine) map[engine.CompKey]DiffLine {
	m := make(map[engine.CompKey]DiffLine, len(ls))
	for _, l := range ls {
		m[l.Key] = l
	}
	return m
}

func unionKeys(a, b map[engine.CompKey]DiffLine) []engine.CompKey {
	keys := make([]engine.CompKey, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	return keys
}

func unionNIKs(a, b map[string]DiffEmp) []string {
	niks := make([]string, 0, len(a)+len(b))
	for k := range a {
		niks = append(niks, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			niks = append(niks, k)
		}
	}
	sort.Strings(niks)
	return niks
}

// TargetFromResults converts dry-run results into the target side.
func TargetFromResults(rs []engine.Result) map[string]DiffEmp {
	out := make(map[string]DiffEmp, len(rs))
	for _, r := range rs {
		e := DiffEmp{Lines: make([]DiffLine, 0, len(r.Lines))}
		for _, l := range r.Lines {
			e.Lines = append(e.Lines, DiffLine{Key: l.Key, Amount: l.Amount})
		}
		if h := r.Header; h != nil {
			e.Head = &HeadTotals{TotTunjangan: h.TotTunjangan, TotPotongan: h.TotPotongan, TakeHome: h.TakeHome}
		}
		out[r.NIK] = e
	}
	return out
}
