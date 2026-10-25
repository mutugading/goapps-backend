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
)

// WarnPPhNoBracket is the warning when legacy SELECT INTO finds no category or bracket (P1 NULL).
const WarnPPhNoBracket = "PPH_NO_BRACKET"

// TERBracket is one PAYROLLSETUP row of a TER category: Min ≤ gross ≤ Max → Rate percent.
type TERBracket struct {
	Min  decimal.Decimal
	Max  decimal.Decimal
	Rate decimal.Decimal // percent, e.g. 0.25
}

// TaxTables are the TER inputs of the rule set (legacy IM_VS_STATIC_VALUE EMPTAX/PAYROLLSETUP).
type TaxTables struct {
	Category map[string]string       // pe_tax_id → A/B/C (EMPTAX.field_01)
	TER      map[string][]TERBracket // category → brackets
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
	for k, l := range ctx.lines { // map order is irrelevant: decimal addition is exact
		if isPPh(k) {
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
// moves the THR PPh from P7 into P1. December (annual) is T014.
type pph21 struct{}

func (pph21) Code() string { return FormulaPPh21 }

func (pph21) Eval(ctx *EvalCtx, _ EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	in := ctx.In
	if in.Period.IsDecember() {
		return nil, "", errPending
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
	cat, okCat := in.Tax.Category[in.Emp.TaxStatus]
	b, okBr := in.Tax.bracket(cat, g)
	if !okCat || !okBr {
		ctx.warn(fmt.Sprintf("%s: %s status pajak %q, bruto %s", WarnPPhNoBracket, in.Emp.NIK, in.Emp.TaxStatus, g))
		return nil, "", nil
	}
	p := money.Round(g.Mul(b.Rate).Shift(-2), 0) // ÷ 100, exact
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

// pphFormulas returns the PPh formulas implemented so far (P5/P0 follow in T015).
func pphFormulas() []Formula { return []Formula{pph21{}} }
