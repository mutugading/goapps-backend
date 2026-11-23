package engine

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine/money"
)

// ConstTaxV2From is the rule constant TAX_V2_FROM (yymm, D-81): from that period a legacy rule set (v1)
// computes PPh with the v2 tax branch only. Absent or 0 = never; a v2 rule set always uses it.
const ConstTaxV2From = "TAX_V2_FROM"

// monthsPerYear is the number of tax months; the v1 job cost cap is MaxMonthly × 12.
const monthsPerYear = 12

// payrollCutoffDay is the last day of a payroll period (periods run 26/MM-1 through 25/MM).
const payrollCutoffDay = 25

// taxV2 reports whether the tax branch of rule v2 applies (D-80 (1), D-81): statutory layers with
// TRUNC −3 (D-29 v2), the v2 P5 composition and PTKP (D-31, D-37b), the job cost capped by the months
// worked, the leaver true-up and P0 = −(P1 + P5). Non-tax items (D1–D19) still follow LegacyCompat.
func taxV2(in Input) bool {
	if !in.Rules.LegacyCompat() {
		return true
	}
	v, ok := in.Rules.Constant(ConstTaxV2From)
	return ok && v.IsPositive() && int64(in.Period.YYMM) >= v.IntPart()
}

// TaxV2 is taxV2 for the loader, which must load the leaver YTD and the annual tables (D-81).
func TaxV2(r RuleSet, yymm int) bool { return taxV2(Input{Rules: r, Period: Period{YYMM: yymm}}) }

// LeaverMonth reports whether resign falls in period p: with tax v2 it is the employee's last tax
// month, so P1/P5 use the annual computation there (D-80 (4a)). December is annual anyway.
func LeaverMonth(p Period, resign *time.Time) bool {
	if resign == nil || p.IsDecember() || p.Start.IsZero() || p.End.IsZero() {
		return false
	}
	return !resign.Before(p.Start) && !resign.After(p.End)
}

// annualMonth reports whether P1/P5 use the annual computation: December, or the leaver month (tax v2).
func annualMonth(in Input) bool {
	return in.Period.IsDecember() || (taxV2(in) && LeaverMonth(in.Period, in.Emp.ResignDate))
}

// workMonths is the number of months worked in the tax year up to the current period (D-80 (4b)):
// the larger of the months with a salary run before this one + 1 (YTD.Months, every placement of the
// tax subject) and the months since the join date (the period holding it, cut-off on the 25th).
// The larger value covers a gap in the migrated runs as well as a placement that changed NIK.
func workMonths(in Input) int {
	last := lastWorkMonth(in)
	n := in.YTD.Months + 1
	if from, ok := payrollMonth(in.Emp.JoinDate, in.Period.Year()); ok {
		n = max(n, last-from+1)
	}
	return max(1, min(n, last))
}

// lastWorkMonth is the current month, or the payroll month of a resign date before this period in the same
// tax year: an employee still active in the master after leaving (rule v1 keeps it, D-55) is not given a job
// cost for months not worked (T033m review).
func lastWorkMonth(in Input) int {
	cur := in.Period.Month()
	r := in.Emp.ResignDate
	if r == nil || in.Period.Start.IsZero() || !r.Before(in.Period.Start) {
		return cur
	}
	if m, ok := payrollMonth(r, in.Period.Year()); ok && m < cur {
		return m
	}
	return cur
}

// payrollMonth is the payroll month (1..12) of year holding date j (cut-off on the 25th), or 1 for a date
// before year. ok is false without a date or for a later year.
func payrollMonth(j *time.Time, year int) (int, bool) {
	if j == nil {
		return 0, false
	}
	y, m := j.Year(), int(j.Month())
	if j.Day() > payrollCutoffDay {
		if m++; m > monthsPerYear {
			m, y = 1, y+1
		}
	}
	switch {
	case y < year:
		return 1, true
	case y == year:
		return m, true
	}
	return 0, false
}

// jobCostCap is the annual job cost cap: MaxMonthly × 12 (v1), or × the months worked (tax v2).
func jobCostCap(in Input) (decimal.Decimal, int) {
	months := monthsPerYear
	if taxV2(in) {
		months = workMonths(in)
	}
	return money.Mul(in.Tax.JobCost.MaxMonthly, decimal.NewFromInt(int64(months))), months
}
