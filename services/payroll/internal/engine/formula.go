package engine

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
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
	fs := append([]Formula{fixedOrProrate{}}, tFormulas()...)
	fs = append(fs, otFormulas()...)
	r, err := NewRegistry(append(fs, pFormulas()...)...)
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

// specFor resolves the formula of a component. T ids follow the legacy branches (tSpecFor,
// D-25), P ids pSpecFor (D-27). ok is false only for an unmapped component, which then
// evaluates with FIXED_OR_PRORATE and Calculate flags the result incomplete.
func specFor(k CompKey) (spec formulaSpec, ok bool) {
	if k.Type == CompP {
		return pSpecFor(k.ID), true
	}
	return tSpecFor(k.ID)
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

// fixedOrProrate is PRO(A): A when STS='F', A/DM × pay_tot when STS='D' (06§3). Legacy has no
// dirumahkan supplement on the component itself (that is T1xx, D-25); STS D without a recap
// row and any other STS give NULL.
type fixedOrProrate struct{}

func (fixedOrProrate) Code() string { return FormulaFixedOrProrate }

func (fixedOrProrate) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	switch c.Sts {
	case stsFull:
		a := c.Amount
		return &a, "", nil
	case stsProrate:
		return prorate(ctx, c.Amount)
	}
	return nil, "", nil
}
