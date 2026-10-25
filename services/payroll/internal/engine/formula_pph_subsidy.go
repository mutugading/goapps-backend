package engine

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// Subsidy kinds and override actions (pyr_subsidy_rule.psr_subsidy_kind, pyr_subsidy_override.pso_action).
const (
	SubsidyFullPPh = "FULL_PPH"

	OverrideInclude = "INCLUDE"
	OverrideExclude = "EXCLUDE"
	OverrideAmount  = "AMOUNT"
)

// Meta keys recorded on the P0 line (psd_meta, 12§4).
const (
	MetaSubsidyRule     = "subsidy_rule_id"
	MetaSubsidyBasis    = "subsidy_basis"
	MetaSubsidyLimit    = "subsidy_limit"
	MetaSubsidyOverride = "subsidy_override"
	// MetaSubsidySource tells where the basis came from (SubsidySrc*). SubsidySrcLockNow marks
	// the first full month of the tax year (rule v2): the runner stores its basis as the lock.
	MetaSubsidySource = "subsidy_basis_source"
)

// Subsidy basis sources (D-30). v1: basis period → previous period → current lines; v2: lock →
// first full month (current lines, locked from now on) → provisional nominal without OT.
const (
	SubsidySrcBasisPeriod = "BASIS_PERIOD"
	SubsidySrcPrevPeriod  = "PREV_PERIOD"
	SubsidySrcCurrent     = "CURRENT"
	SubsidySrcLocked      = "LOCKED"
	SubsidySrcLockNow     = "LOCK_NOW"
	SubsidySrcProvisional = "PROVISIONAL"
)

// ErrSubsidyKind is returned for a subsidy kind or override action the engine does not know.
var ErrSubsidyKind = errors.New("engine: unsupported subsidy kind or override action")

// SubsidyRule is one ACTIVE pyr_subsidy_rule (12§4.2), restricted to what rule v1 uses. Rule v1
// migrates MGT_PAYROLL_SUBSIDI_PPH rows with YN = 'Y' into rules per period and division; legacy
// matches yymm + division only (D-30), so Depts stays empty for migrated rules.
type SubsidyRule struct {
	ID          int64
	Name        string
	Priority    int // lowest first; the first rule that matches wins
	ValidFrom   int // yymm
	ValidTo     int // yymm, 0 = open
	Divisions   []string
	Depts       []string
	BasisPeriod int             // psr_basis_period_yymm, e.g. 202601 (D17); 0 = current period
	Limit       decimal.Decimal // psr_limit_amount (SUBSIDIPPH 10.000.000)
	Kind        string          // FULL_PPH
}

// matches reports whether the rule applies to the period and employee (empty list = all).
func (r SubsidyRule) matches(in Input) bool {
	y := in.Period.YYMM
	if y < r.ValidFrom || (r.ValidTo != 0 && y > r.ValidTo) {
		return false
	}
	if len(r.Divisions) > 0 && !slices.Contains(r.Divisions, in.Division) {
		return false
	}
	return len(r.Depts) == 0 || slices.Contains(r.Depts, in.Emp.Dept)
}

// SubsidyOverride is pyr_subsidy_override of the period for the employee (approved only).
type SubsidyOverride struct {
	Action string          // INCLUDE / EXCLUDE / AMOUNT
	Amount decimal.Decimal // AMOUNT: subsidy granted, P0 = −Amount
	Reason string
}

// Subsidy is the P0 input, loaded with the rule set (not queried by the engine). The loader
// fills the fields of the active rule version (D-30).
type Subsidy struct {
	Rules    []SubsidyRule
	Override *SubsidyOverride
	// Rule v1. Basis is Σ DETAIL Tunjangan tax_subsidy = Y of the rule basis period (202601);
	// PrevBasis the same sum of the latest DETAIL period after it and before the run period.
	// nil = no DETAIL there; nil or zero falls through to the next step (legacy NVL(·,0) = 0).
	Basis     *decimal.Decimal
	PrevBasis *decimal.Decimal
	// Rule v2. Locked is Σ DETAIL tax_subsidy = Y (OT included) of the first full month of the
	// tax year, LockedYYMM that month; nil while the employee has no full month yet. The runner
	// stores it from the P0 line whose source is SubsidySrcLockNow.
	Locked     *decimal.Decimal
	LockedYYMM int
}

// isOTSubsidyID reports the overtime T ids (D-30 lists 6/8/20–31/37/44): the v2 provisional basis of
// the join month leaves them out.
func isOTSubsidyID(id int) bool {
	return id == 6 || id == 8 || (id >= 20 && id <= 31) || id == 37 || id == 44
}

// withSubsidyComponent adds P0 for rule v2 when a rule or override applies: v2 drops the legacy
// P0-row gate, an individual exclusion is an EXCLUDE override (D-30). Rule v1 keeps the gate:
// P0 is evaluated only when the loader passes the employee's P0 row (frz = 2).
func withSubsidyComponent(in Input, comps []EmpComponent) []EmpComponent {
	if in.Rules.LegacyCompat() {
		return comps
	}
	if _, ok := firstSubsidyRule(in); !ok && in.Subsidy.Override == nil {
		return comps
	}
	k := CompKey{CompP, 0}
	for _, c := range comps {
		if c.Key == k {
			return comps
		}
	}
	return append(comps, EmpComponent{Key: k, Sts: stsFull})
}

// pphSubsidy is P0 (06§6.4, 12§4): −P1 when the first matching rule's basis ≤ limit, else 0.
// An override decides before the rules.
type pphSubsidy struct{}

func (pphSubsidy) Code() string { return FormulaPPhSubsidy }

func (pphSubsidy) Eval(ctx *EvalCtx, _ EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	in := ctx.In
	zero := decimal.Zero
	p1 := ctx.lines[CompKey{CompP, 1}].Amount // nil: P1 absent or NULL
	neg := func() *decimal.Decimal {          // legacy V_PPH × −1 keeps NULL
		if p1 == nil {
			return nil
		}
		v := p1.Neg()
		return &v
	}
	if o := in.Subsidy.Override; o != nil {
		ctx.setMeta(MetaSubsidyOverride, o.Action)
		remark := "Override : " + o.Action + " | " + o.Reason
		switch o.Action {
		case OverrideInclude:
			return neg(), remark, nil
		case OverrideExclude:
			return &zero, remark, nil
		case OverrideAmount:
			v := o.Amount.Neg()
			return &v, remark, nil
		}
		return nil, "", fmt.Errorf("%w: override %q", ErrSubsidyKind, o.Action)
	}
	r, ok := firstSubsidyRule(in)
	if !ok { // legacy: no SUBSIDI_PPH row → YN 'N' → 0
		return &zero, "", nil
	}
	if r.Kind != SubsidyFullPPh {
		return nil, "", fmt.Errorf("%w: rule %d kind %q", ErrSubsidyKind, r.ID, r.Kind)
	}
	basis, src := subsidyBasis(ctx)
	ctx.setMeta(MetaSubsidyRule, strconv.FormatInt(r.ID, 10))
	ctx.setMeta(MetaSubsidyBasis, basis.String())
	ctx.setMeta(MetaSubsidyLimit, r.Limit.String())
	ctx.setMeta(MetaSubsidySource, src)
	remark := fmt.Sprintf("Aturan : %s | Basis : %s (%s) | Batas : %s", r.Name, thousands(basis), src, thousands(r.Limit))
	if basis.GreaterThan(r.Limit) {
		return &zero, remark, nil
	}
	return neg(), remark, nil
}

// firstSubsidyRule returns the matching rule with the lowest priority (ties: lowest id).
func firstSubsidyRule(in Input) (SubsidyRule, bool) {
	var best SubsidyRule
	found := false
	for _, r := range in.Subsidy.Rules {
		if !r.matches(in) {
			continue
		}
		if !found || r.Priority < best.Priority || (r.Priority == best.Priority && r.ID < best.ID) {
			best, found = r, true
		}
	}
	return best, found
}

// subsidyBasis returns the P0 basis and its source (D-30).
func subsidyBasis(ctx *EvalCtx) (decimal.Decimal, string) {
	if ctx.In.Rules.LegacyCompat() {
		return subsidyBasisV1(ctx)
	}
	return subsidyBasisV2(ctx)
}

// subsidyBasisV1 is the legacy order of FNC_MGT_PAYROLL_PPH: DETAIL 202601, else the latest DETAIL
// before the period, else Σ the current taxable lines with tax_subsidy = Y (prorated).
func subsidyBasisV1(ctx *EvalCtx) (decimal.Decimal, string) {
	s := ctx.In.Subsidy
	if s.Basis != nil && !s.Basis.IsZero() {
		return *s.Basis, SubsidySrcBasisPeriod
	}
	if s.PrevBasis != nil && !s.PrevBasis.IsZero() {
		return *s.PrevBasis, SubsidySrcPrevPeriod
	}
	return currentSubsidyLines(ctx), SubsidySrcCurrent
}

// subsidyBasisV2 uses the locked basis of the first full month of the tax year. Without one, a
// full month computes it from the current lines (OT included) and becomes the lock; the join month
// uses the provisional basis without OT. A lock of this or a later period is ignored so that a
// recalculation of the locking month uses its latest lines.
func subsidyBasisV2(ctx *EvalCtx) (decimal.Decimal, string) {
	s := ctx.In.Subsidy
	if s.Locked != nil && s.LockedYYMM < ctx.In.Period.YYMM {
		return *s.Locked, SubsidySrcLocked
	}
	if fullMonth(ctx) {
		return currentSubsidyLines(ctx), SubsidySrcLockNow
	}
	return provisionalSubsidyBasis(ctx), SubsidySrcProvisional
}

// provisionalSubsidyBasis is the join-month basis (D-30): Σ the taxable tax_subsidy = Y T lines
// without OT, where a prorated PRO(A) component counts at its nominal (value before proration) and
// any other formula component at its computed line.
func provisionalSubsidyBasis(ctx *EvalCtx) decimal.Decimal {
	var sum decimal.Decimal
	for _, c := range ctx.In.Components {
		if c.Key.Type != CompT || !c.Tax || !c.TaxSubsidy || isOTSubsidyID(c.Key.ID) {
			continue
		}
		if spec, _ := specFor(c.Key); spec.Code == FormulaFixedOrProrate && c.Sts == stsProrate {
			sum = sum.Add(c.Amount)
			continue
		}
		sum = sum.Add(ctx.lines[c.Key].AmountOrZero())
	}
	return sum
}

// fullMonth is the D-30 full month: joined on or before the period start (dates compared without
// time of day) and not prorated (base salary STS F, or pay_tot = DM). A missing join date counts
// as joined earlier.
func fullMonth(ctx *EvalCtx) bool {
	in := ctx.In
	if j := in.Emp.JoinDate; j != nil && dateOf(*j).After(dateOf(in.Period.Start)) {
		return false
	}
	b := ctx.Basis
	return b.GapokSts == stsFull || b.rekapOrZero(in, colPayTot).Equal(b.DaysMonth)
}

// dateOf drops the time of day.
func dateOf(t time.Time) time.Time {
	y, m, dd := t.Date()
	return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
}

// currentSubsidyLines is Σ the computed taxable T lines with tax_subsidy = Y (NULL as zero).
func currentSubsidyLines(ctx *EvalCtx) decimal.Decimal {
	var sum decimal.Decimal
	for k, l := range ctx.lines {
		if k.Type == CompT && l.Tax && l.TaxSubsidy {
			sum = sum.Add(l.AmountOrZero())
		}
	}
	return sum
}
