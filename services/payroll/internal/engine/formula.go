package engine

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Formula codes known to the skeleton. Component-specific formulas are added by later tasks.
const (
	FormulaFixedOrProrate = "FIXED_OR_PRORATE"
	FormulaPPh21          = "PPH21"
	FormulaPPhCompany     = "PPH_COMPANY"
	FormulaPPhSubsidy     = "PPH_SUBSIDY"
)

// Params are per-component formula parameters (e.g. recap column, multiplier).
type Params map[string]string

// Formula evaluates one component. A nil amount is a legacy NULL result (D10).
// The returned amount is the raw value; Calculate applies ROUND(·, 0).
type Formula interface {
	Code() string
	Eval(ctx *EvalCtx, c EmpComponent, p Params) (amt *decimal.Decimal, remark string, err error)
}

// Registry maps formula codes to implementations. It is immutable once built.
type Registry struct {
	byCode map[string]Formula
}

// NewRegistry builds a registry; duplicate or empty codes are rejected.
func NewRegistry(fs ...Formula) (Registry, error) {
	m := make(map[string]Formula, len(fs))
	for _, f := range fs {
		code := f.Code()
		if code == "" {
			return Registry{}, errors.New("engine: formula with empty code")
		}
		if _, dup := m[code]; dup {
			return Registry{}, fmt.Errorf("engine: duplicate formula %q", code)
		}
		m[code] = f
	}
	return Registry{byCode: m}, nil
}

// Get returns the formula for code.
func (r Registry) Get(code string) (Formula, bool) {
	f, ok := r.byCode[code]
	return f, ok
}

// DefaultRegistry returns the formulas implemented so far.
func DefaultRegistry() Registry {
	r, err := NewRegistry(fixedOrProrate{})
	if err != nil {
		panic(err) // static set, cannot fail
	}
	return r
}

// formulaSpec binds a component to a formula code and its parameters.
type formulaSpec struct {
	Code   string
	Params Params
}

// specFor resolves the formula of a component. ok is false when the component has no
// explicit mapping yet; it then evaluates with the default (amount as-is, 06§3 "lainnya")
// and Calculate flags the result incomplete. Per-ID mappings (06§3, 06§4) arrive with
// the formula tasks.
func specFor(k CompKey) (spec formulaSpec, ok bool) {
	if k.Type == CompP {
		switch k.ID {
		case 0:
			return formulaSpec{Code: FormulaPPhSubsidy}, true
		case 1:
			return formulaSpec{Code: FormulaPPh21}, true
		case 5:
			return formulaSpec{Code: FormulaPPhCompany}, true
		}
		return formulaSpec{Code: FormulaFixedOrProrate}, false
	}
	switch k.ID {
	case 1, 2, 3, 4, 5, 7, 8, 9, 19, 20:
		return formulaSpec{Code: FormulaFixedOrProrate}, true
	}
	return formulaSpec{Code: FormulaFixedOrProrate}, false
}

// isPPh reports whether k is evaluated in the PPh phase (P1, P5, P0, in that order).
func isPPh(k CompKey) bool { return k.Type == CompP && (k.ID == 0 || k.ID == 1 || k.ID == 5) }

// pphRank gives the evaluation order inside the PPh phase: P1 → P5 → P0.
func pphRank(k CompKey) int {
	switch k.ID {
	case 1:
		return 0
	case 5:
		return 1
	default:
		return 2
	}
}

// EvalCtx is what formulas can read: the input, the bases and lines already computed.
type EvalCtx struct {
	In    Input
	Basis Basis
	lines map[CompKey]Line
}

// Line returns an already computed line (e.g. PPh formulas reading T results).
func (c *EvalCtx) Line(k CompKey) (Line, bool) {
	l, ok := c.lines[k]
	return l, ok
}

// fixedOrProrate is PRO(A): A when STS='F', A/DM × pay_tot when STS='D' (06§3).
// The D-with-dirumahkan supplement is [V] and not implemented yet.
type fixedOrProrate struct{}

func (fixedOrProrate) Code() string { return FormulaFixedOrProrate }

func (fixedOrProrate) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if c.Sts != "D" {
		a := c.Amount
		return &a, "", nil
	}
	q, err := money.Div(c.Amount, ctx.Basis.DaysMonth)
	if err != nil {
		return nil, "", err
	}
	payTot := ctx.In.Att.Get("pay_tot")
	a := q.Mul(payTot)
	return &a, fmt.Sprintf("%s / %s × %s hari", c.Amount, ctx.Basis.DaysMonth, payTot), nil
}
