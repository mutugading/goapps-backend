package engine

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Overtime and shift formula codes (06§3, legacy FNC_MGT_PAYROLL_COMPNT_CALC, D-25).
const (
	FormulaOTInwork    = "OT_INWORK"     // T6
	FormulaOTInworkAdj = "OT_INWORK_ADJ" // T36 (D9)
	FormulaOTFixed     = "OT_FIXED"      // T8
	FormulaOTAddition  = "OT_ADDITION"   // T20
	FormulaOTHours     = "OT_HOURS"      // T21–31
	FormulaShift       = "SHIFT"         // T34, T35
	FormulaShiftAdd    = "SHIFT_ADD"     // T134, T135
	FormulaOTOprAdd    = "OT_OPR_ADD"    // T136
)

// Recap columns read by the overtime/shift formulas (R.x / RA.x).
const (
	colOtiwDecr  = "otiw_decr"
	colShift     = "shift"
	colShiftAdd  = "shift_add"
	colPayTotAdd = "pay_tot_add"
)

// Formula parameter keys for OT_HOURS.
const (
	paramMult    = "mult"     // multiplier for JAKARTA and non-Executive
	paramMultExc = "mult_exc" // Executive multiplier; absent = no Executive branch (NULL, D11)
)

// otHoursSpec gives the recap column and multipliers of T21–31. Executive has a branch
// only for the NAS ids 26–28, always ×2 (D11); T29 is ×2 in legacy, not ×4.
func otHoursSpec(id int) (formulaSpec, bool) {
	type row struct{ col, mult, multExc string }
	rows := map[int]row{
		21: {"lembur_1", "1.5", ""}, 22: {"lembur_2", "2", ""},
		23: {"lembur_off_1", "2", ""}, 24: {"lembur_off_2", "3", ""}, 25: {"lembur_off_3", "4", ""},
		26: {"lembur_nas_1", "2", "2"}, 27: {"lembur_nas_2", "3", "2"}, 28: {"lembur_nas_3", "4", "2"},
		29: {"lembur_nasp_1", "2", ""}, 30: {"lembur_nasp_2", "3", ""}, 31: {"lembur_nasp_3", "4", ""},
	}
	r, ok := rows[id]
	if !ok {
		return formulaSpec{}, false
	}
	p := Params{paramCol: r.col, paramMult: r.mult}
	if r.multExc != "" {
		p[paramMultExc] = r.multExc
	}
	return formulaSpec{Code: FormulaOTHours, Params: p}, true
}

// otSpecFor maps the overtime/shift T ids.
func otSpecFor(id int) (formulaSpec, bool) {
	switch id {
	case 6:
		return formulaSpec{Code: FormulaOTInwork}, true
	case 8:
		return formulaSpec{Code: FormulaOTFixed}, true
	case 20:
		return formulaSpec{Code: FormulaOTAddition}, true
	case 34, 35:
		return formulaSpec{Code: FormulaShift}, true
	case 36:
		return formulaSpec{Code: FormulaOTInworkAdj}, true
	case 134, 135:
		return formulaSpec{Code: FormulaShiftAdd}, true
	case 136:
		return formulaSpec{Code: FormulaOTOprAdd}, true
	}
	return otHoursSpec(id)
}

// otRate is the legacy hourly rate of 06§2 (V_RATEOTJKT/EXC/OPR ÷ 173). ok is false when
// the legacy SUM has no rows (NULL), or when the employee falls in no legacy branch:
// Executive without an Executive branch, or a NULL level compared with != 'Executive'.
func otRate(ctx *EvalCtx, excBranch bool) (decimal.Decimal, bool) {
	b := ctx.Basis
	switch {
	case isJakarta(ctx.In.Division), excBranch && isExecutive(ctx.In.Emp.Level):
		return b.RateOT, b.hasRateOT
	case isExecutive(ctx.In.Emp.Level):
		return decimal.Zero, false
	case !excBranch && strings.TrimSpace(ctx.In.Emp.Level) == "": // NULL != 'Executive' is unknown
		return decimal.Zero, false
	}
	return b.RateOT, b.hasRateOT
}

// stsFullOrNull handles the STS branches shared by the OT/shift formulas: F gives A,
// any STS other than F/D gives NULL; done is false only for STS D.
func stsFullOrNull(c EmpComponent) (*decimal.Decimal, bool) {
	switch c.Sts {
	case stsFull:
		a := c.Amount
		return &a, true
	case stsProrate:
		return nil, false
	}
	return nil, true
}

// otHours is T21–31: F → A, D → rate/HM × R.col × mult.
type otHours struct{}

func (otHours) Code() string { return FormulaOTHours }

func (otHours) Eval(ctx *EvalCtx, c EmpComponent, p Params) (*decimal.Decimal, string, error) {
	if a, done := stsFullOrNull(c); done {
		return a, "", nil
	}
	_, excBranch := p[paramMultExc]
	rate, ok := otRate(ctx, excBranch)
	if !ok {
		return nil, "", nil
	}
	hours, ok := ctx.rekap(p[paramCol])
	if !ok {
		return nil, "", nil
	}
	key := paramMult
	if excBranch && isExecutive(ctx.In.Emp.Level) && !isJakarta(ctx.In.Division) {
		key = paramMultExc
	}
	mult, err := decParam(p, key)
	if err != nil {
		return nil, "", err
	}
	a := rate.Mul(hours).Mul(mult)
	return &a, fmt.Sprintf("Rate per jam %s × %s jam × %s", money.Round(rate, 3), hours, mult), nil
}

// otAddition is T20: F → ADD, D → rate/HM × ADD (ADD holds hours); NULL without an addition row.
// Every level has a branch here, so a NULL level falls to the operator rate.
type otAddition struct{}

func (otAddition) Code() string { return FormulaOTAddition }

func (otAddition) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	add, rem, ok := ctx.addition(c.Key)
	if !ok {
		return nil, "", nil
	}
	switch c.Sts {
	case stsFull:
		return &add, rem, nil
	case stsProrate:
		rate, ok := otRate(ctx, true)
		if !ok {
			return nil, "", nil
		}
		a := rate.Mul(add)
		return &a, strings.TrimSpace(fmt.Sprintf("Rate per jam %s × %s jam %s", money.Round(rate, 3), add, rem)), nil
	}
	return nil, "", nil
}

// inworkRule is one legacy OT-in-work variant selected by the component amount (D9):
// TOT = hours × weeks × 52 / 12, ratio = (TOT − hours × fail × perFail) / TOT, 0 above maxFail.
type inworkRule struct {
	amount, hours           string
	weeks, perFail, maxFail int64
}

// inworkRules returns the variants per formula: T6 knows 0.0877/0.075/0.9017, T36 only 0.0127.
func inworkRules(code string) []inworkRule {
	if code == FormulaOTInworkAdj {
		return []inworkRule{{amount: "0.0127", hours: "0.75", weeks: 4, perFail: 4, maxFail: 4}}
	}
	return []inworkRule{
		{amount: "0.0877", hours: "3.5", weeks: 1, perFail: 1, maxFail: 4},
		{amount: "0.075", hours: "0.75", weeks: 4, perFail: 4, maxFail: 4},
		{amount: "0.9017", hours: "6", weeks: 6, perFail: 1, maxFail: 26},
	}
}

// inworkRuleFor picks the variant whose amount equals a (legacy IF P_AMOUNT = …).
func inworkRuleFor(code string, a decimal.Decimal) (inworkRule, bool) {
	for _, r := range inworkRules(code) {
		if a.Equal(decimal.RequireFromString(r.amount)) {
			return r, true
		}
	}
	return inworkRule{}, false
}

// ratio computes V_OTINWORK with the legacy operation order.
func (r inworkRule) ratio(fail decimal.Decimal) (decimal.Decimal, error) {
	if fail.GreaterThan(decimal.NewFromInt(r.maxFail)) {
		return decimal.Zero, nil
	}
	h := decimal.RequireFromString(r.hours)
	tot, err := money.Div(h.Mul(decimal.NewFromInt(r.weeks)).Mul(decimal.NewFromInt(52)), decimal.NewFromInt(12))
	if err != nil {
		return decimal.Zero, err
	}
	return money.Div(tot.Sub(h.Mul(fail).Mul(decimal.NewFromInt(r.perFail))), tot)
}

// otInwork is T6 / T36: F → A, D → RATEOTOPR × A × ratio, where RATEOTOPR is Σ T{1,7,55}
// (not divided by HM) for every division and level. An amount without a legacy variant
// leaves V_OTINWORK NULL, so the result is NULL.
type otInwork struct{ code string }

func (f otInwork) Code() string { return f.code }

func (f otInwork) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if a, done := stsFullOrNull(c); done {
		return a, "", nil
	}
	rule, ok := inworkRuleFor(f.code, c.Amount)
	if !ok || !ctx.Basis.hasOTOpr {
		return nil, "", nil
	}
	ratio, err := rule.ratio(ctx.Basis.rekapOrZero(ctx.In, colOtiwDecr))
	if err != nil {
		return nil, "", err
	}
	a := ctx.Basis.OTOpr.Mul(c.Amount).Mul(ratio)
	return &a, fmt.Sprintf("Rate %s × %s%% × %s%%", ctx.Basis.OTOpr, c.Amount.Shift(2), money.Round(ratio.Shift(2), 3)), nil
}

// otFixed is T8: F → A; D → A when otiw_decr is 0, else A/WD × (WD − otiw_decr).
// NULL when the period/division has no ATTD_DAYS row.
type otFixed struct{}

func (otFixed) Code() string { return FormulaOTFixed }

func (otFixed) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if a, done := stsFullOrNull(c); done {
		return a, "", nil
	}
	if ctx.In.Workdays == nil {
		return nil, "", nil
	}
	wd := decimal.NewFromInt(int64(*ctx.In.Workdays))
	decr := ctx.Basis.rekapOrZero(ctx.In, colOtiwDecr)
	if decr.IsZero() { // legacy: WD = WD − NVL(otiw_decr, 0)
		a := c.Amount
		return &a, "Fix overtime " + c.Amount.String(), nil
	}
	q, err := money.Div(c.Amount, wd)
	if err != nil {
		return nil, "", err
	}
	days := wd.Sub(decr)
	a := q.Mul(days)
	return &a, fmt.Sprintf("%s / %s × %s hari", c.Amount, wd, days), nil
}

// shift is SHIFT (T34/35, R.shift) and SHIFT_ADD (T134/135, RA.shift_add): F → A, D → A × days.
type shift struct{ code string }

func (f shift) Code() string { return f.code }

func (f shift) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if a, done := stsFullOrNull(c); done {
		return a, "", nil
	}
	var days decimal.Decimal
	var ok bool
	if f.code == FormulaShiftAdd {
		days, ok = ctx.In.AttAdd.Get(colShiftAdd), ctx.In.AttAdd != nil // C_ATT_REKAP_ADD has no T1 join
	} else {
		days, ok = ctx.rekap(colShift)
	}
	if !ok {
		return nil, "", nil
	}
	a := c.Amount.Mul(days)
	return &a, fmt.Sprintf("%s × %s hari", c.Amount, days), nil
}

// otOprAdd is T136: F → A, D → RATEOTOPR / DM × A × RA.pay_tot_add. Legacy assigns its
// description string to V_OUTPUT (ORA-06502); the intended amount is used instead (D-25).
type otOprAdd struct{}

func (otOprAdd) Code() string { return FormulaOTOprAdd }

func (otOprAdd) Eval(ctx *EvalCtx, c EmpComponent, _ Params) (*decimal.Decimal, string, error) {
	if a, done := stsFullOrNull(c); done {
		return a, "", nil
	}
	if ctx.In.AttAdd == nil || !ctx.Basis.hasOTOpr {
		return nil, "", nil
	}
	perDay, err := money.Div(ctx.Basis.OTOpr, ctx.Basis.DaysMonth)
	if err != nil {
		return nil, "", err
	}
	days := ctx.In.AttAdd.Get(colPayTotAdd)
	a := perDay.Mul(c.Amount).Mul(days)
	return &a, fmt.Sprintf("Rate per hari %s × %s%% × %s hari", money.Round(perDay, 3), c.Amount.Shift(2), days), nil
}

// otFormulas returns the overtime/shift formulas.
func otFormulas() []Formula {
	return []Formula{
		otInwork{FormulaOTInwork}, otInwork{FormulaOTInworkAdj}, otFixed{}, otAddition{}, otHours{},
		shift{FormulaShift}, shift{FormulaShiftAdd}, otOprAdd{},
	}
}
