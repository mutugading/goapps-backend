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
	TetapD      decimal.Decimal // prorated variant for STS='D'; equals Tetap otherwise
	JanBase     decimal.Decimal // A(T,901)
	RateOT      decimal.Decimal // overtime hourly rate, per division/level
	DaysPeriod  int             // end − start + 1; 0 when the period dates are unset
	DaysMonth   decimal.Decimal // DM
	HoursMonth  decimal.Decimal // HM
	rateJamBase decimal.Decimal
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

// isJakarta accepts the spec name and the IAM division code (JKT).
func isJakarta(div string) bool {
	d := strings.ToUpper(strings.TrimSpace(div))
	return d == "JAKARTA" || d == "JKT"
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
	if c, ok := comps[CompKey{CompT, idGapok}]; ok {
		b.GapokSts = c.Sts
	}
	b.Tetap = amountOf(comps, tetapIDs()...)
	b.JanBase = amountOf(comps, idJanBase)
	b.DaysPeriod = daysBetween(in.Period.Start, in.Period.End)

	// TETAP_D: each component prorated amount/DM × pay_tot [V: legacy C_GPTUNJTETAP branch D].
	b.TetapD = b.Tetap
	if b.GapokSts == "D" { // STS(T,1) is the single source (06§2)
		payTot := in.Att.Get("pay_tot")
		sum := decimal.Zero
		for _, id := range tetapIDs() {
			c, ok := comps[CompKey{CompT, id}]
			if !ok {
				continue
			}
			q, err := money.Div(c.Amount, dm)
			if err != nil {
				return Basis{}, err
			}
			sum = sum.Add(q.Mul(payTot))
		}
		b.TetapD = sum
	}

	// RATE_OT: JAKARTA T1/HM; Executive Σ T{1,2,3,4}/HM; others Σ T{1,7,55}/HM.
	var otNum decimal.Decimal
	switch {
	case isJakarta(in.Division):
		otNum = amountOf(comps, idGapok)
	case isExecutive(in.Emp.Level):
		otNum = amountOf(comps, otExecutiveIDs()...)
	default:
		otNum = amountOf(comps, otDefaultIDs()...)
	}
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
