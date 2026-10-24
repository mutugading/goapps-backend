package master

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/component"
)

// Legacy component codes touched by TRG_MGT_PAYROLL_EMP_MAST (02§5.1, D-21).
const (
	codeOtinwork   = "T006" // T6: OT in work rate.
	codeShiftAdj   = "T036" // T36: shift adjustment.
	codeTunjShiftN = "T034" // T34: shift allowance for non-staff.
	codeTunjShiftS = "T035" // T35: shift allowance for staff.
)

var (
	nonStaffDrop = []string{"T002", "T003", "T004"} // removed for non-staff, kept for staff.
	staffGroup   = "STAFF"
)

// ruleCodes lists the codes a level rule may need to resolve in the catalog.
func ruleCodes() []string {
	return append([]string{codeOtinwork, codeShiftAdj, codeTunjShiftN, codeTunjShiftS}, nonStaffDrop...)
}

// levelPlan is the outcome of comparing the attachments an employee holds with
// the ones a level asks for.
type levelPlan struct {
	Changes       []component.Change
	SkippedManual []string // codes of template components held as MANUAL; untouched.
}

func sameDecimal(a, b string) bool {
	da, errA := decimal.NewFromString(orZero(a))
	db, errB := decimal.NewFromString(orZero(b))
	return errA == nil && errB == nil && da.Equal(db)
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func sameAttachment(a, b component.Attachment) bool {
	return a.Sts == b.Sts && a.Tax == b.Tax && a.TaxPaidCompany == b.TaxPaidCompany && a.Frz == b.Frz && sameDecimal(a.Amount, b.Amount)
}

// desiredFromTemplate builds the TEMPLATE attachments a level asks for: the level
// template plus the per-level overrides of pyr_level_rule (02§5.1, D-21). catalog
// maps code to component and must hold every code of ruleCodes() when rule is set.
func desiredFromTemplate(template []component.Attachment, rule *component.LevelRule, catalog map[string]component.CatalogEntry) (map[int64]component.Attachment, error) {
	want := make(map[int64]component.Attachment, len(template))
	for _, t := range template {
		t.Source, t.Frz = component.SourceTemplate, 2
		want[t.Sysid] = t
	}
	if rule == nil {
		return want, nil
	}
	if err := applyLevelRule(want, rule, catalog); err != nil {
		return nil, err
	}
	return want, nil
}

func ruleEntry(catalog map[string]component.CatalogEntry, code string) (component.Attachment, error) {
	c, ok := catalog[code]
	if !ok {
		return component.Attachment{}, validation("level", "komponen "+code+" yang dibutuhkan aturan level tidak ada/aktif di company ini")
	}
	return component.Attachment{Sysid: c.Sysid, Code: c.Code, Name: c.Name, Sts: c.DefaultSts, Source: component.SourceTemplate}, nil
}

func upsertRule(want map[int64]component.Attachment, catalog map[string]component.CatalogEntry, code, amount string, frz int) error {
	a, err := ruleEntry(catalog, code)
	if err != nil {
		return err
	}
	if cur, ok := want[a.Sysid]; ok { // keep template sts/tax, override rate and freeze.
		a = cur
	}
	a.Amount, a.Frz = amount, frz
	want[a.Sysid] = a
	return nil
}

func dropRule(want map[int64]component.Attachment, catalog map[string]component.CatalogEntry, codes ...string) {
	for _, code := range codes {
		if c, ok := catalog[code]; ok {
			delete(want, c.Sysid)
		}
	}
}

// setFrz sets the freeze flag of the wanted attachment of code, if present.
func setFrz(want map[int64]component.Attachment, catalog map[string]component.CatalogEntry, code string, frz int) {
	c, ok := catalog[code]
	if !ok {
		return
	}
	if a, in := want[c.Sysid]; in {
		a.Frz = frz
		want[c.Sysid] = a
	}
}

func applyStaff(want map[int64]component.Attachment, rule *component.LevelRule, catalog map[string]component.CatalogEntry) error {
	dropRule(want, catalog, codeOtinwork, codeTunjShiftN)
	if err := upsertRule(want, catalog, codeTunjShiftS, rule.TunjshiftRate, rule.TunjshiftSts); err != nil {
		return err
	}
	for _, code := range nonStaffDrop {
		setFrz(want, catalog, code, 2)
	}
	return nil
}

func applyNonStaff(want map[int64]component.Attachment, rule *component.LevelRule, catalog map[string]component.CatalogEntry) error {
	dropRule(want, catalog, nonStaffDrop...)
	dropRule(want, catalog, codeTunjShiftS)
	if err := upsertRule(want, catalog, codeOtinwork, rule.OtinworkRate, rule.OtinworkSts); err != nil {
		return err
	}
	return upsertRule(want, catalog, codeTunjShiftN, rule.TunjshiftRate, rule.TunjshiftSts)
}

// applyLevelRule mirrors the trigger: non-staff upsert T6/T34 and drop T2/T3/T4;
// staff upsert T35 and drop T6/T34 (T2/T3/T4 become active); T36 follows SHIFTADJ.
func applyLevelRule(want map[int64]component.Attachment, rule *component.LevelRule, catalog map[string]component.CatalogEntry) error {
	apply := applyNonStaff
	if rule.Group == staffGroup {
		apply = applyStaff
	}
	if err := apply(want, rule, catalog); err != nil {
		return err
	}
	setFrz(want, catalog, codeShiftAdj, rule.ShiftadjSts)
	return nil
}

// planLevel diffs current attachments against want. MANUAL rows are never changed
// or removed (12§2.2 rule 4); a wanted component held as MANUAL is reported as skipped.
func planLevel(current []component.Attachment, want map[int64]component.Attachment, effectiveYYMM int) levelPlan {
	var p levelPlan
	have := make(map[int64]component.Attachment, len(current))
	for _, c := range current {
		have[c.Sysid] = c
	}
	for _, w := range want {
		cur, held := have[w.Sysid]
		switch {
		case !held:
			w.ValidFrom = effectiveYYMM
			p.Changes = append(p.Changes, component.Change{Action: component.ActionAdd, Sysid: w.Sysid, Code: w.Code, Name: w.Name, After: &w})
		case cur.Source == component.SourceManual:
			p.SkippedManual = append(p.SkippedManual, cur.Code)
		case !sameAttachment(cur, w):
			before, after := cur, w
			after.ValidFrom, after.ValidTo, after.Name = effectiveYYMM, 0, cur.Name
			p.Changes = append(p.Changes, component.Change{Action: component.ActionChange, Sysid: w.Sysid, Code: cur.Code, Name: cur.Name, Before: &before, After: &after})
		}
	}
	for _, c := range current {
		if _, wanted := want[c.Sysid]; !wanted && c.Source == component.SourceTemplate {
			before := c
			p.Changes = append(p.Changes, component.Change{Action: component.ActionRemove, Sysid: c.Sysid, Code: c.Code, Name: c.Name, Before: &before})
		}
	}
	slices.SortFunc(p.Changes, func(a, b component.Change) int { return cmp.Compare(a.Sysid, b.Sysid) })
	slices.Sort(p.SkippedManual)
	return p
}
