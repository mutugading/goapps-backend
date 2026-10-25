package engine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// ConstAllowNegativePPh is the rule constant allow_negative_pph (06§6.1): 1 lets P1 go below
// zero when PPh already paid on THR exceeds the period PPh; 0 floors P1 at zero. Absent = 1.
const ConstAllowNegativePPh = "ALLOW_NEGATIVE_PPH"

// Ids subtracted from the PPh gross: P4 Absensi, P8 Deduction Impact To PPh (06§5).
const (
	idAbsen           = 4
	idDeductionImpact = 8
)

// PPh errors and warnings.
var (
	// ErrNoTaxTable is returned when P1 needs TER but the input carries no category or bracket table.
	// Unlike a missing bracket for one employee (NULL + warning), an empty table is a loader fault
	// that affects every employee, so the run fails.
	ErrNoTaxTable = errors.New("engine: PPh TER table (EMPTAX/PAYROLLSETUP) missing from input")
	// errPending marks a PPh branch not implemented yet; Calculate skips the line as incomplete.
	errPending = errors.New("engine: formula branch not implemented yet")
	// ErrNoAnnualTaxTable is returned in December when PTKP, BIAYAJABATAN or TARIF is missing.
	ErrNoAnnualTaxTable = errors.New("engine: PPh annual table (PTKP/BIAYAJABATAN/TARIF) missing from input")
)

// WarnPPhNoBracket is the warning when legacy SELECT INTO finds no category or bracket (P1 NULL).
const WarnPPhNoBracket = "PPH_NO_BRACKET"

// TERBracket is one PAYROLLSETUP row of a TER category: Min ≤ gross ≤ Max → Rate percent.
type TERBracket struct {
	Min  decimal.Decimal
	Max  decimal.Decimal
	Rate decimal.Decimal // percent, e.g. 0.25
}

// TaxTables are the PPh inputs of the rule set (legacy IM_VS_STATIC_VALUE EMPTAX/PAYROLLSETUP).
type TaxTables struct {
	Category map[string]string       // pe_tax_id → A/B/C (EMPTAX.field_01)
	TER      map[string][]TERBracket // category → brackets
	// December (annual) tables.
	PTKP        map[string]decimal.Decimal // pe_tax_id → monthly PTKP (PAYROLLSETUP TK, K0, …)
	JobCost     JobCost                    // PAYROLLSETUP BIAYAJABATAN
	Progressive []TaxLayer                 // PAYROLLSETUP TARIF5 … TARIF35, ascending
}

// bracket returns the bracket of cat containing gross (legacy BETWEEN, both ends inclusive).
func (t TaxTables) bracket(cat string, gross decimal.Decimal) (TERBracket, bool) {
	for _, b := range t.TER[cat] {
		if gross.GreaterThanOrEqual(b.Min) && gross.LessThanOrEqual(b.Max) {
			return b, true
		}
	}
	return TERBracket{}, false
}

// ThrTax is the THR/bonus paid in the same tax month (06§6.1, 14§2.5). Zero outside THR months.
type ThrTax struct {
	Gross   decimal.Decimal // THR_GROSS: Σ taxable v_pyr_thr_detail, yymm(pay_date) = period
	PPhPaid decimal.Decimal // PPH_THR_PAID: PPh already withheld on that THR
}

// pphGross is the legacy non-December gross of FNC_MGT_PAYROLL_PPH (D-28): Σ ROUND(·,0) of every
// component row with tax = 2, T and P alike (additions only through their component row), minus
// P4 and P8. ok is false when legacy finds no row at all (no taxable component, no P4/P8).
func pphGross(ctx *EvalCtx) (gross decimal.Decimal, ok bool) {
	return pphGrossExcept(ctx, nil)
}

// pphGrossExcept is pphGross without the taxable rows whose id is in skip (December drops 75).
// A skipped row still exists in legacy (it only drops out of the sum), so it counts for ok.
func pphGrossExcept(ctx *EvalCtx, skip map[int]bool) (gross decimal.Decimal, ok bool) {
	return pphGrossWhere(ctx, func(k CompKey, _ Line) bool { return skip[k.ID] }, true)
}

// pphGrossWhere sums like pphGross, leaving out the taxable rows for which drop is true.
// droppedCounts tells whether such a row still makes legacy find a row (ok).
func pphGrossWhere(ctx *EvalCtx, drop func(CompKey, Line) bool, droppedCounts bool) (gross decimal.Decimal, ok bool) {
	for k, l := range ctx.lines { // map order is irrelevant: decimal addition is exact
		if isPPh(k) {
			continue
		}
		if l.Tax && drop(k, l) {
			ok = ok || droppedCounts
			continue
		}
		if l.Tax {
			gross, ok = gross.Add(l.AmountOrZero()), true
		}
		if k == (CompKey{CompP, idAbsen}) || k == (CompKey{CompP, idDeductionImpact}) {
			gross, ok = gross.Sub(l.AmountOrZero()), true
		}
	}
	return gross, ok
}

// pph21 is P1 (06§6.1, D-28). Non-December: ROUND(G × TER(G) / 100, 0) with G = gross + THR_GROSS,
// minus PPH_THR_PAID. Only that subtraction can go below zero; it is floored at zero unless
// ALLOW_NEGATIVE_PPH. Legacy has no THR term here (manual T75/P75/P7 additions instead); 06§6.1 v1
// moves the THR PPh from P7 into P1. December is the annual reconciliation (pph21Annual).
type pph21 struct{}

func (pph21) Code() string { return FormulaPPh21 }

func (pph21) Eval(ctx *EvalCtx, _ EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	in := ctx.In
	if in.Period.IsDecember() {
		return pph21Annual(ctx)
	}
	gross, ok := pphGross(ctx)
	thr := in.Thr
	if !ok && thr.Gross.IsZero() && thr.PPhPaid.IsZero() {
		return nil, "", nil
	}
	if len(in.Tax.Category) == 0 || len(in.Tax.TER) == 0 {
		return nil, "", ErrNoTaxTable
	}
	g := gross.Add(thr.Gross)
	p, cat, b, okBr := terTax(ctx, g)
	if !okBr {
		return nil, "", nil
	}
	remark := fmt.Sprintf("Kategori : %s | Penghasilan : %s - %s | Tarif : %s %%", cat, thousands(b.Min), thousands(b.Max), b.Rate)
	if !thr.Gross.IsZero() || !thr.PPhPaid.IsZero() {
		p = p.Sub(thr.PPhPaid)
		remark += fmt.Sprintf(" | THR : %s | PPh THR : %s", thousands(thr.Gross), thousands(thr.PPhPaid))
	}
	if p.IsNegative() && !allowNegativePPh(in.Rules) {
		p = decimal.Zero
	}
	return &p, remark, nil
}

// terTax is ROUND(g × TER(category, g) / 100, 0). ok is false, with a PPH_NO_BRACKET warning,
// when the tax status has no category or g falls in no bracket (legacy SELECT INTO → NULL).
func terTax(ctx *EvalCtx, g decimal.Decimal) (p decimal.Decimal, cat string, b TERBracket, ok bool) {
	in := ctx.In
	cat, okCat := in.Tax.Category[in.Emp.TaxStatus]
	b, okBr := in.Tax.bracket(cat, g)
	if !okCat || !okBr {
		ctx.warn(fmt.Sprintf("%s: %s status pajak %q, bruto %s", WarnPPhNoBracket, in.Emp.NIK, in.Emp.TaxStatus, g))
		return decimal.Zero, cat, b, false
	}
	return money.Round(g.Mul(b.Rate).Shift(-2), 0), cat, b, true // ÷ 100, exact
}

// allowNegativePPh reads ALLOW_NEGATIVE_PPH; the default allows it for parity (06§6.1).
func allowNegativePPh(r RuleSet) bool {
	v, ok := r.Constant(ConstAllowNegativePPh)
	return !ok || !v.IsZero()
}

// thousands formats an integer amount like Oracle 'FM999G999G999G999G990' (e.g. 5,400,001).
func thousands(v decimal.Decimal) string {
	s := v.Round(0).Abs().String()
	var b strings.Builder
	if v.Round(0).IsNegative() {
		b.WriteByte('-')
	}
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pphFormulas returns the PPh formulas P1, P5 and P0.
func pphFormulas() []Formula { return []Formula{pph21{}, pphCompany{}, pphSubsidy{}} }
