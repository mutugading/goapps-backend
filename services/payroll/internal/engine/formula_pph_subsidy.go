package engine

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

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

// Subsidy is the P0 input, loaded with the rule set (not queried by the engine).
type Subsidy struct {
	Rules    []SubsidyRule
	Override *SubsidyOverride
	// Basis is Σ Tunjangan tax_subsidy = Y of DETAIL in the rule basis period; nil when the
	// employee has no DETAIL there. A nil or zero basis falls back to the current lines (06§6.4).
	Basis *decimal.Decimal
}

// pphSubsidy is P0 (06§6.4, 12§4): −P1 when the first matching rule's basis ≤ limit, else 0.
// Legacy computes P0 only for employees with a P0 component row; the engine does the same since
// it evaluates the components it is given. An override decides before the rules.
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
	basis := subsidyBasis(ctx)
	ctx.setMeta(MetaSubsidyRule, strconv.FormatInt(r.ID, 10))
	ctx.setMeta(MetaSubsidyBasis, basis.String())
	ctx.setMeta(MetaSubsidyLimit, r.Limit.String())
	remark := fmt.Sprintf("Aturan : %s | Basis : %s | Batas : %s", r.Name, thousands(basis), thousands(r.Limit))
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

// subsidyBasis is the basis-period amount, or Σ current taxable lines with tax_subsidy = Y when
// that is absent or zero (legacy NVL(V_SUBSIDY,0) = 0). The intermediate legacy step (latest
// DETAIL before the period, D-30) follows in T015a.
func subsidyBasis(ctx *EvalCtx) decimal.Decimal {
	if b := ctx.In.Subsidy.Basis; b != nil && !b.IsZero() {
		return *b
	}
	var sum decimal.Decimal
	for k, l := range ctx.lines {
		if k.Type == CompT && l.Tax && l.TaxSubsidy {
			sum = sum.Add(l.AmountOrZero())
		}
	}
	return sum
}
