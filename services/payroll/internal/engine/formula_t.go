package engine

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Tunjangan (T) formula codes without overtime/shift (formula_ot.go) (06§3, legacy FNC_MGT_PAYROLL_COMPNT_CALC, D-25).
const (
	FormulaAddition    = "ADDITION"
	FormulaSubsidiPPh  = "SUBSIDI_PPH_JKT"
	FormulaPremiHadir  = "PREMI_HADIR"
	FormulaBeras       = "BERAS"
	FormulaDirumahkan  = "DIRUMAHKAN"
	FormulaSakit       = "SAKIT"
	FormulaNotWorking  = "NW"
	FormulaBPJSNaker   = "BPJS_NAKER"
	FormulaBPJSKes     = "BPJS_KES"
	FormulaBPJSPensiun = "BPJS_PENSIUN"
	// FormulaLegacyNull is a component id without a legacy branch: V_OUTPUT stays NULL (D-25).
	FormulaLegacyNull = "LEGACY_NULL"
)

// Rule constant keys used by T formulas (06§1, 06§4).
const (
	ConstBerasDays = "BERAS_DAYS"
	ConstNakMin    = "NAKMIN"
	ConstKesMin    = "KESMIN"
	ConstKesMax    = "KESMAX"
	ConstPenMin    = "PENMIN"
	ConstPenMax    = "PENMAX"
)

// Formula parameter keys.
const (
	paramCol      = "col"      // recap column (R.x)
	paramPct      = "pct"      // multiplier, e.g. 0.75
	paramSchedule = "schedule" // pyr_stg_sch_rekap schedule code (NW / NWS)
)

// Component statuses.
const (
	stsFull    = "F"
	stsProrate = "D"
)

// rekap returns NVL-free MPAR_col: ok is false when legacy has no recap row (column NULL).
func (c *EvalCtx) rekap(col string) (decimal.Decimal, bool) {
	if !c.Basis.hasRekap {
		return decimal.Zero, false
	}
	return c.In.Att.Get(col), true
}

// addition returns ADD(k) and its remark; ok is false when there is no addition row (NULL).
// pyr_addition has one row per (yymm, nik, type, id, division), so the sum equals the single
// legacy FETCH; summing only guards callers that pass several rows (D-25).
func (c *EvalCtx) addition(k CompKey) (decimal.Decimal, string, bool) {
	adds, ok := c.In.Additions[k]
	if !ok || len(adds) == 0 {
		return decimal.Zero, "", false
	}
	sum := decimal.Zero
	rems := make([]string, 0, len(adds))
	for _, a := range adds {
		sum = sum.Add(a.Amount)
		if a.Remark != "" {
			rems = append(rems, a.Remark)
		}
	}
	return sum, strings.Join(rems, "; "), true
}

// component returns the employee component k.
func (c *EvalCtx) component(k CompKey) (EmpComponent, bool) {
	for _, e := range c.In.Components {
		if e.Key == k {
			return e, true
		}
	}
	return EmpComponent{}, false
}

// optConstant is a legacy PAYROLLSETUP cursor: no row leaves the variable at its default 0.
func optConstant(r RuleSet, key string) decimal.Decimal {
	if v, ok := r.Constant(key); ok {
		return v
	}
	return decimal.Zero
}

func decParam(p Params, key string) (decimal.Decimal, error) {
	v, err := decimal.NewFromString(p[key])
	if err != nil {
		return decimal.Zero, fmt.Errorf("engine: param %s=%q: %w", key, p[key], err)
	}
	return v, nil
}

// prorate is x/DM × pay_tot; nil when legacy has no recap row.
func prorate(ctx *EvalCtx, x decimal.Decimal) (*decimal.Decimal, string, error) {
	payTot, ok := ctx.rekap(colPayTot)
	if !ok {
		return nil, "", nil
	}
	q, err := money.Div(x, ctx.Basis.DaysMonth)
	if err != nil {
		return nil, "", err
	}
	a := q.Mul(payTot)
	return &a, fmt.Sprintf("%s / %s × %s hari", x, ctx.Basis.DaysMonth, payTot), nil
}

// addition is PRO(ADD(T,i)): F → ADD, D → ADD/DM × pay_tot; NULL without an addition row.
type addition struct{}

func (addition) Code() string { return FormulaAddition }

func (addition) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	add, rem, ok := ctx.addition(c.Key)
	if !ok {
		return nil, "", nil
	}
	switch c.Sts {
	case stsFull:
		return &add, rem, nil
	case stsProrate:
		a, r, err := prorate(ctx, add)
		if a == nil || err != nil {
			return a, r, err
		}
		return a, strings.TrimSpace(r + " " + rem), nil
	}
	return nil, "", nil
}

// subsidiPPh is T10: ROUND((ADD(T,38) + (A74 − A74/25 × (alpa+ijin))) / 0.95 × 5/100, 0), STS F only.
// Each term exists only when its legacy row exists; no row at all gives NULL.
type subsidiPPh struct{}

func (subsidiPPh) Code() string { return FormulaSubsidiPPh }

func (subsidiPPh) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if c.Sts != stsFull {
		return nil, "", nil
	}
	var sum *decimal.Decimal
	if add, _, ok := ctx.addition(CompKey{CompT, 38}); ok {
		sum = &add
	}
	if t74, ok := ctx.component(CompKey{CompT, 74}); ok && ctx.In.HasRekap() {
		bd, err := constant(ctx.In.Rules, ConstBerasDays)
		if err != nil {
			return nil, "", err
		}
		// Legacy C_SUBSIDI_PPH_JKT uses recap alpa+ijin (no T1 join), not Daily.BerasFail.
		fail := ctx.In.Att.Get(colAlpa).Add(ctx.In.Att.Get(colIjin))
		q, err := money.Div(t74.Amount, bd)
		if err != nil {
			return nil, "", err
		}
		v := t74.Amount.Sub(q.Mul(fail))
		if sum != nil {
			v = v.Add(*sum)
		}
		sum = &v
	}
	if sum == nil {
		return nil, "", nil
	}
	gross, err := money.Div(*sum, decimal.RequireFromString("0.95"))
	if err != nil {
		return nil, "", err
	}
	v, err := money.Div(gross.Mul(decimal.NewFromInt(5)), decimal.NewFromInt(100))
	if err != nil {
		return nil, "", err
	}
	v = money.Round(v, 0)
	return &v, fmt.Sprintf("Subsidi PPh 5%% atas %s / 0,95", sum), nil
}

// premiHadir is T40: NULL when premi_fail > 0 (D10); else F → A, D → A/DM × day_work.
type premiHadir struct{}

func (premiHadir) Code() string { return FormulaPremiHadir }

func (premiHadir) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if ctx.In.Daily.PremiFail != 0 {
		return nil, fmt.Sprintf("Gagal premi %d hari", ctx.In.Daily.PremiFail), nil
	}
	switch c.Sts {
	case stsFull:
		a := c.Amount
		return &a, "", nil
	case stsProrate:
		dayWork, ok := ctx.rekap(colDayWork)
		if !ok {
			return nil, "", nil
		}
		q, err := money.Div(c.Amount, ctx.Basis.DaysMonth)
		if err != nil {
			return nil, "", err
		}
		a := q.Mul(dayWork)
		return &a, fmt.Sprintf("%s / %s × %s hari kerja", c.Amount, ctx.Basis.DaysMonth, dayWork), nil
	}
	return nil, "", nil
}

// beras is T74: F → A, D → A − A/BD × beras_fail.
type beras struct{}

func (beras) Code() string { return FormulaBeras }

func (beras) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	switch c.Sts {
	case stsFull:
		a := c.Amount
		return &a, "", nil
	case stsProrate:
		bd, err := constant(ctx.In.Rules, ConstBerasDays)
		if err != nil {
			return nil, "", err
		}
		fail := decimal.NewFromInt(int64(ctx.In.Daily.BerasFail))
		q, err := money.Div(c.Amount, bd)
		if err != nil {
			return nil, "", err
		}
		a := c.Amount.Sub(q.Mul(fail))
		return &a, fmt.Sprintf("%s − %s / %s × %s hari", c.Amount, c.Amount, bd, fail), nil
	}
	return nil, "", nil
}

// daysPct covers DIRUMAHKAN (101–107, 155) and SAKIT (4xx): F → A × pct,
// D → A/DM × R.col × pct. A is the component's own amount (D-25).
type daysPct struct{ code string }

func (f daysPct) Code() string { return f.code }

func (f daysPct) Eval(ctx *EvalCtx, c EmpComponent, p Params) (*decimal.Decimal, string, error) {
	pct, err := decParam(p, paramPct)
	if err != nil {
		return nil, "", err
	}
	switch c.Sts {
	case stsFull:
		a := c.Amount.Mul(pct)
		return &a, fmt.Sprintf("%s × %s", c.Amount, pct), nil
	case stsProrate:
		days, ok := ctx.rekap(p[paramCol])
		if !ok {
			return nil, "", nil
		}
		q, err := money.Div(c.Amount, ctx.Basis.DaysMonth)
		if err != nil {
			return nil, "", err
		}
		a := q.Mul(days).Mul(pct)
		return &a, fmt.Sprintf("%s / %s × %s hari × %s", c.Amount, ctx.Basis.DaysMonth, days, pct), nil
	}
	return nil, "", nil
}

// notWorking is NW (201–207, 255) and NWS (301): F → A/2, D → A/2/DM × SCH; NULL without a schedule row.
type notWorking struct{}

func (notWorking) Code() string { return FormulaNotWorking }

func (notWorking) Eval(ctx *EvalCtx, c EmpComponent, p Params) (*decimal.Decimal, string, error) {
	half, err := money.Div(c.Amount, decimal.NewFromInt(2))
	if err != nil {
		return nil, "", err
	}
	switch c.Sts {
	case stsFull:
		return &half, fmt.Sprintf("%s / 2", c.Amount), nil
	case stsProrate:
		sch, ok := ctx.In.Sch[p[paramSchedule]]
		if !ok {
			return nil, "", nil
		}
		q, err := money.Div(half, ctx.Basis.DaysMonth)
		if err != nil {
			return nil, "", err
		}
		a := q.Mul(sch)
		return &a, fmt.Sprintf("%s / 2 / %s × %s hari %s", c.Amount, ctx.Basis.DaysMonth, sch, p[paramSchedule]), nil
	}
	return nil, "", nil
}

// bpjs is the employer BPJS allowance: clamp(base, min, max) × A where the base is TETAP
// (STS F) or TETAP_D (STS D); BPJS_KES uses JAN_BASE in January. A missing PAYROLLSETUP
// constant is 0, like the legacy variable default.
type bpjs struct {
	code     string
	minKey   string
	maxKey   string // "" = no upper bound (NAKER)
	janBased bool
}

func (f bpjs) Code() string { return f.code }

func (f bpjs) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	b := ctx.Basis
	var base decimal.Decimal
	var present bool
	jan := f.janBased && ctx.In.Period.IsJanuary()
	switch {
	case c.Sts != stsFull && c.Sts != stsProrate:
		return nil, "", nil
	case jan && c.Sts == stsFull:
		base, present = b.JanBase, b.hasJanBase
	case jan:
		base, present = b.JanBaseD, b.hasJanBase
	case c.Sts == stsFull:
		base, present = b.Tetap, b.hasTetap
	default:
		base, present = b.TetapD, b.hasTetap
	}
	if !present { // NULL base: every comparison is unknown, ELSE branch gives NULL × A
		return nil, "", nil
	}
	// Same IF/ELSIF order as legacy: max is checked before min.
	lo := optConstant(ctx.In.Rules, f.minKey)
	switch {
	case f.maxKey != "" && base.GreaterThan(optConstant(ctx.In.Rules, f.maxKey)):
		base = optConstant(ctx.In.Rules, f.maxKey)
	case base.LessThan(lo):
		base = lo
	}
	a := base.Mul(c.Amount)
	return &a, fmt.Sprintf("Basis %s × %s", base, c.Amount), nil
}

// legacyNull is a component without a legacy branch: the line exists with a NULL amount.
type legacyNull struct{}

func (legacyNull) Code() string { return FormulaLegacyNull }

func (legacyNull) Eval(*EvalCtx, EmpComponent, Params) (*decimal.Decimal, string, error) {
	return nil, "", nil
}

// tFormulas returns the non-overtime T formulas.
func tFormulas() []Formula {
	return []Formula{
		addition{}, subsidiPPh{}, premiHadir{}, beras{},
		daysPct{FormulaDirumahkan}, daysPct{FormulaSakit}, notWorking{},
		bpjs{code: FormulaBPJSNaker, minKey: ConstNakMin},
		bpjs{code: FormulaBPJSKes, minKey: ConstKesMin, maxKey: ConstKesMax, janBased: true},
		bpjs{code: FormulaBPJSPensiun, minKey: ConstPenMin, maxKey: ConstPenMax},
		legacyNull{},
	}
}

// tSpecFor maps a T component id to its formula (legacy FNC_MGT_PAYROLL_COMPNT_CALC, D-25).
// Every id is mapped: ids without a legacy branch get LEGACY_NULL.
func tSpecFor(id int) (formulaSpec, bool) {
	if s, ok := otSpecFor(id); ok {
		return s, true
	}
	switch {
	case id >= 1 && id <= 9, id == 49, id == 55, id == 57, id == 73: // 6, 8 are overtime (otSpecFor)
		return formulaSpec{Code: FormulaFixedOrProrate}, true
	case id == 10:
		return formulaSpec{Code: FormulaSubsidiPPh}, true
	case id >= 11 && id <= 13:
		return formulaSpec{Code: FormulaBPJSNaker}, true
	case id == 15:
		return formulaSpec{Code: FormulaBPJSKes}, true
	case id == 17:
		return formulaSpec{Code: FormulaBPJSPensiun}, true
	case isAdditionID(id):
		return formulaSpec{Code: FormulaAddition}, true
	case id == 40:
		return formulaSpec{Code: FormulaPremiHadir}, true
	case id == 74:
		return formulaSpec{Code: FormulaBeras}, true
	}
	if s, ok := tFamilySpec(id); ok {
		return s, true
	}
	return formulaSpec{Code: FormulaLegacyNull}, true
}

// familyID reports whether id is first + k for a legacy family base k in {0..4, 6} or
// is the T55 member (e.g. 101–105, 107, 155 for first=101, t55=155).
func familyID(id, first, t55 int) bool {
	switch id - first {
	case 0, 1, 2, 3, 4, 6:
		return true
	}
	return id == t55
}

// tFamilySpec maps the 1xx (dirumahkan), 2xx/301 (not working) and 4xx (sakit) families.
func tFamilySpec(id int) (formulaSpec, bool) {
	switch {
	case familyID(id, 101, 155):
		return formulaSpec{Code: FormulaDirumahkan, Params: Params{paramCol: colDirumahkan, paramPct: "0.5"}}, true
	case familyID(id, 201, 255):
		return formulaSpec{Code: FormulaNotWorking, Params: Params{paramSchedule: "NW"}}, true
	case id == 301:
		return formulaSpec{Code: FormulaNotWorking, Params: Params{paramSchedule: "NWS"}}, true
	case familyID(id, 401, 455):
		return formulaSpec{Code: FormulaSakit, Params: Params{paramCol: "sakit_75", paramPct: "0.75"}}, true
	case familyID(id, 411, 456):
		return formulaSpec{Code: FormulaSakit, Params: Params{paramCol: "sakit_50", paramPct: "0.5"}}, true
	case familyID(id, 421, 457):
		return formulaSpec{Code: FormulaSakit, Params: Params{paramCol: "sakit_25", paramPct: "0.25"}}, true
	}
	return formulaSpec{}, false
}

// isAdditionID lists the T ids whose amount comes from pyr_addition (PRO(ADD)).
func isAdditionID(id int) bool {
	switch {
	case id == 19, id == 32, id == 33, id == 38, id >= 41 && id <= 48, id >= 70 && id <= 72, id >= 75 && id <= 78:
		return true
	}
	return false
}
