package recon

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Exclusion reasons of D-47 (opsi A). An excluded NIK stays in the report (lines, heads, warnings) but is left out
// of the parity criteria: FORMULA/ROUNDING counts, Σ take-home and the per-NIK header check.
const (
	// ExclSyntheticMain: the legacy MAIN is the synthetic one of D-35.2 (DETAIL without MAIN, defect D2).
	ExclSyntheticMain = "SYNTHETIC_MAIN_D2"
	// ExclMasterDrift: the current master differs from what legacy calculated the period with.
	ExclMasterDrift = "MASTER_DRIFT"
)

// Exclusions lists every exclusion reason in report order.
var Exclusions = []string{ExclSyntheticMain, ExclMasterDrift}

// Warning codes of excluded NIKs.
const (
	CodeExclSyntheticMain = "EXCLUDED_" + ExclSyntheticMain
	CodeExclMasterDrift   = "EXCLUDED_" + ExclMasterDrift
)

// DriftInput holds the master attributes that tell whether the legacy result of a NIK is comparable (D-47).
type DriftInput struct {
	// SyntheticMain marks NIKs whose LEGACY MAIN was synthesized by the ETL (D-35.2).
	SyntheticMain map[string]bool
	// Inactive marks NIKs whose master pe_sts_payroll is not 2 now (the target does not calculate them).
	Inactive map[string]bool
	// Missing marks NIKs with a LEGACY MAIN but no master row in the division (ETL master / NIK mapping).
	Missing map[string]bool
	// Frozen lists, per NIK, the components with frz_flag <> 2 in the master now (validity window of the period).
	Frozen map[string]map[engine.CompKey]bool
}

// ExclusionStat is the aggregate of one exclusion reason.
type ExclusionStat struct {
	NIK   int `json:"nik"`
	Lines int `json:"lines"` // lines with Δ ≠ 0
	// OtherLines are Δ ≠ 0 lines of a frozen-component drift on components other than the frozen one(s): they
	// may hide a formula difference (dependent BPJS/PPh lines are expected here), so they are shown separately.
	OtherLines     int             `json:"other_lines"`
	HeadMismatch   int             `json:"head_mismatch"`
	TakeHomeLegacy decimal.Decimal `json:"take_home_legacy"`
	TakeHomeTarget decimal.Decimal `json:"take_home_target"`
	TakeHomeDelta  decimal.Decimal `json:"take_home_delta"`
}

// exclusion is the D-47 verdict of one NIK. cause = the frozen components of a frz drift (nil = whole NIK).
type exclusion struct {
	reason, detail string
	cause          map[engine.CompKey]bool
}

// exclusionOf returns the D-47 reason a NIK is left out of the parity criteria (reason "" = included).
// The synthetic MAIN wins: its attributes come from the current master, so drift is not meaningful there.
func exclusionOf(d DriftInput, nik string, leg DiffEmp, comps []engine.EmpComponent) exclusion {
	switch {
	case d.SyntheticMain[nik]:
		return exclusion{reason: ExclSyntheticMain, detail: "MAIN legacy sintetis (D-35.2, D2)"}
	case leg.Head != nil && d.Missing[nik]:
		return exclusion{reason: ExclMasterDrift, detail: "tidak ada di master divisi ini, legacy punya MAIN"}
	case leg.Head != nil && d.Inactive[nik]:
		return exclusion{reason: ExclMasterDrift, detail: "pe_sts_payroll master sekarang ≠ 2, legacy punya MAIN"}
	}
	if keys := frozenPaid(leg.Lines, comps, d.Frozen[nik]); len(keys) > 0 {
		names := make([]string, 0, len(keys))
		for _, k := range sortedKeys(keys) {
			names = append(names, k.String())
		}
		return exclusion{reason: ExclMasterDrift, cause: keys,
			detail: "komponen " + strings.Join(names, ",") + " frz ≠ 2 di master sekarang, legacy punya DETAIL"}
	}
	return exclusion{}
}

func sortedKeys(m map[engine.CompKey]bool) []engine.CompKey {
	out := make([]engine.CompKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	return out
}

// frozenPaid lists the non-zero legacy lines whose component is frozen in the current master and not attached active.
func frozenPaid(legacy []DiffLine, comps []engine.EmpComponent, frozen map[engine.CompKey]bool) map[engine.CompKey]bool {
	if len(frozen) == 0 {
		return nil
	}
	active := make(map[engine.CompKey]bool, len(comps))
	for _, c := range comps {
		if !c.Frozen {
			active[c.Key] = true
		}
	}
	var out map[engine.CompKey]bool
	for _, l := range legacy {
		if frozen[l.Key] && !active[l.Key] && !orZero(l.Amount).IsZero() {
			if out == nil {
				out = map[engine.CompKey]bool{}
			}
			out[l.Key] = true
		}
	}
	return out
}

// addExcluded adds an excluded NIK: Δ ≠ 0 lines get the reason's category, heads are listed with the reason,
// and the take-home goes to the reason's aggregate instead of the division totals.
func (rep *DiffReport) addExcluded(nik string, ex exclusion, leg, tgt DiffEmp) {
	reason := ex.reason
	s := &rep.Summary
	countNIK(s, leg, tgt)
	st := s.Excluded[reason]
	st.NIK++
	cat := CatMasterDrift
	if reason == ExclSyntheticMain {
		cat = CatKnownDefect
	}
	lm, tm := indexLines(leg.Lines), indexLines(tgt.Lines)
	for _, k := range unionKeys(lm, tm) {
		l, t := lm[k], tm[k]
		delta := orZero(t.Amount).Sub(orZero(l.Amount))
		if delta.IsZero() {
			s.Categories[CatMatch]++
			continue
		}
		st.Lines++
		if ex.cause != nil && !ex.cause[k] {
			st.OtherLines++
		}
		rep.addLine(LineDiff{NIK: nik, Component: k.String(), Legacy: l.Amount, Target: t.Amount, Delta: delta, Category: cat})
	}
	if leg.Head != nil {
		st.TakeHomeLegacy = st.TakeHomeLegacy.Add(leg.Head.TakeHome)
	}
	if tgt.Head != nil {
		st.TakeHomeTarget = st.TakeHomeTarget.Add(tgt.Head.TakeHome)
	}
	st.TakeHomeDelta = st.TakeHomeTarget.Sub(st.TakeHomeLegacy)
	if !sameHead(leg.Head, tgt.Head) {
		st.HeadMismatch++
		rep.Heads = append(rep.Heads, HeadDiff{NIK: nik, Legacy: leg.Head, Target: tgt.Head, Excluded: reason})
	}
	s.Excluded[reason] = st
	code := CodeExclMasterDrift
	if reason == ExclSyntheticMain {
		code = CodeExclSyntheticMain
	}
	rep.Warnings = append(rep.Warnings, run.Issue{NIK: nik, Code: code, Message: ex.detail})
}
