package engine

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// Rule constant keys used by the basis calculation (06§1).
const (
	ConstDaysMonth  = "DAYS_MONTH"
	ConstHoursMonth = "HOURS_MONTH"
)

// Component IDs (type T) that make up the bases (06§2).
const (
	idGapok   = 1
	idJanBase = 901
)

// Component id sets; functions (not package vars) so nothing global is mutable (06§9).
func tetapIDs() []int       { return []int{1, 2, 3, 4, 7, 9, 55} }
func otExecutiveIDs() []int { return []int{1, 2, 3, 4} }
func otDefaultIDs() []int   { return []int{1, 7, 55} }

// ErrMissingConstant is returned when a required rule constant is absent or invalid.
var ErrMissingConstant = errors.New("engine: missing rule constant")

// Basis holds the values computed once per employee (06§2).
type Basis struct {
	Gapok       decimal.Decimal // Σ A(T,1)
	GapokSts    string          // STS(T,1); "" when T1 absent
	Tetap       decimal.Decimal // Σ A(T,{1,2,3,4,7,9,55})
	TetapD      decimal.Decimal // TETAP/DM × pay_tot + dirumahkan × TETAP/DM/2 (D-25)
	JanBase     decimal.Decimal // A(T,901)
	JanBaseD    decimal.Decimal // JAN_BASE prorated like TETAP_D
	RateOT      decimal.Decimal // overtime hourly rate, per division/level
	OTOpr       decimal.Decimal // legacy V_RATEOTOPR: Σ A(T,{1,7,55}), monthly (not ÷ HM), for T6/T36/T136
	DaysPeriod  int             // end − start + 1; 0 when the period dates are unset
	DaysMonth   decimal.Decimal // DM
	HoursMonth  decimal.Decimal // HM
	rateJamBase decimal.Decimal
	hasRekap    bool // a legacy recap row exists: recap present and T1 present
	hasTetap    bool // legacy SUM over no rows is NULL
	hasJanBase  bool
	hasRateOT   bool // the rate cursor of RateOT has rows (legacy SUM is NULL otherwise)
	hasOTOpr    bool
}

// anyOf reports whether any of the T components with the given ids exists.
func anyOf(comps map[CompKey]EmpComponent, ids ...int) bool {
	for _, id := range ids {
		if _, ok := comps[CompKey{CompT, id}]; ok {
			return true
		}
	}
	return false
}

// Recap columns read by the bases and formulas (06§1 R.x).
const (
	colPayTot     = "pay_tot"
	colDayWork    = "day_work"
	colDirumahkan = "dirumahkan"
	colAlpa       = "alpa"
	colIjin       = "ijin"
)

// rekapOrZero is NVL(MPAR_col, 0) with the legacy row semantics of hasRekap.
func (b Basis) rekapOrZero(in Input, col string) decimal.Decimal {
	if !b.hasRekap {
		return decimal.Zero
	}
	return in.Att.Get(col)
}

// prorateBasis is legacy V_GPTUNJTETAP_D: base/DM × pay_tot + dirumahkan × base/DM/2,
// in that operation order and without intermediate rounding (06§1).
func prorateBasis(base, dm, payTot, dirumahkan decimal.Decimal) (decimal.Decimal, error) {
	perDay, err := money.Div(base, dm)
	if err != nil {
		return decimal.Zero, err
	}
	half, err := money.Div(dirumahkan.Mul(base), dm)
	if err != nil {
		return decimal.Zero, err
	}
	if half, err = money.Div(half, decimal.NewFromInt(2)); err != nil {
		return decimal.Zero, err
	}
	return perDay.Mul(payTot).Add(half), nil
}

// amountOf sums the amounts of the T components with the given ids.
func amountOf(comps map[CompKey]EmpComponent, ids ...int) decimal.Decimal {
	sum := decimal.Zero
	for _, id := range ids {
		if c, ok := comps[CompKey{CompT, id}]; ok {
			sum = sum.Add(c.Amount)
		}
	}
	return sum
}

func constant(r RuleSet, key string) (decimal.Decimal, error) {
	v, ok := r.Constant(key)
	if !ok || v.Sign() <= 0 {
		return decimal.Zero, fmt.Errorf("%w: %s", ErrMissingConstant, key)
	}
	return v, nil
}

// daysBetween returns the inclusive number of calendar days from start to end.
func daysBetween(start, end time.Time) int {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	s := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	e := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(e.Sub(s).Hours()/24) + 1
}

func isExecutive(level string) bool { return strings.EqualFold(strings.TrimSpace(level), "executive") }

// divJakarta is the legacy division name with its own overtime rate.
const divJakarta = "JAKARTA"

// isJakarta accepts the spec name and the IAM division code (JKT).
func isJakarta(div string) bool {
	d := strings.ToUpper(strings.TrimSpace(div))
	return d == divJakarta || d == "JKT"
}

// computeBasis calculates the bases of 06§2 once per employee.
// Division results carry no intermediate rounding (06§1); rounding happens per component.
func computeBasis(in Input) (Basis, error) {
	dm, err := constant(in.Rules, ConstDaysMonth)
	if err != nil {
		return Basis{}, err
	}
	hm, err := constant(in.Rules, ConstHoursMonth)
	if err != nil {
		return Basis{}, err
	}

	comps := make(map[CompKey]EmpComponent, len(in.Components))
	for _, c := range in.Components {
		comps[c.Key] = c
	}

	b := Basis{DaysMonth: dm, HoursMonth: hm}
	b.Gapok = amountOf(comps, idGapok)
	gapok, hasT1 := comps[CompKey{CompT, idGapok}]
	if hasT1 {
		b.GapokSts = gapok.Sts
	}
	// Legacy C_ATT_REKAP joins COMPNT_EMP T1: without T1 every MPAR_* value is NULL.
	b.hasRekap = in.HasRekap() && hasT1
	b.Tetap = amountOf(comps, tetapIDs()...)
	b.JanBase = amountOf(comps, idJanBase)
	b.hasTetap = anyOf(comps, tetapIDs()...)
	b.hasJanBase = anyOf(comps, idJanBase)
	b.DaysPeriod = daysBetween(in.Period.Start, in.Period.End)

	// TETAP_D / JAN_BASE_D (D-25): always computed; used by components whose own STS is 'D'.
	if b.TetapD, err = prorateBasis(b.Tetap, dm, b.rekapOrZero(in, colPayTot), b.rekapOrZero(in, colDirumahkan)); err != nil {
		return Basis{}, err
	}
	if b.JanBaseD, err = prorateBasis(b.JanBase, dm, b.rekapOrZero(in, colPayTot), b.rekapOrZero(in, colDirumahkan)); err != nil {
		return Basis{}, err
	}

	// RATE_OT: JAKARTA T1/HM; Executive Σ T{1,2,3,4}/HM; others Σ T{1,7,55}/HM.
	otIDs := otDefaultIDs()
	switch {
	case isJakarta(in.Division):
		otIDs = []int{idGapok}
	case isExecutive(in.Emp.Level):
		otIDs = otExecutiveIDs()
	}
	otNum := amountOf(comps, otIDs...)
	b.hasRateOT = anyOf(comps, otIDs...)
	b.OTOpr = amountOf(comps, otDefaultIDs()...)
	b.hasOTOpr = anyOf(comps, otDefaultIDs()...)
	if b.RateOT, err = money.Div(otNum, hm); err != nil {
		return Basis{}, err
	}

	// Header rate_perjam ignores the JAKARTA special case (06§7).
	hdrNum := amountOf(comps, otDefaultIDs()...)
	if isExecutive(in.Emp.Level) {
		hdrNum = amountOf(comps, otExecutiveIDs()...)
	}
	if b.rateJamBase, err = money.Div(hdrNum, hm); err != nil {
		return Basis{}, err
	}
	return b, nil
}
