// Package run implements the payroll run use cases. T016b: the input loader that turns the
// master, attendance snapshot and addition tables of one (company, division, period) into
// engine inputs (03§3 steps 2-3).
package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Loader errors.
var (
	ErrNoSnapshot          = errors.New("run: no locked attendance snapshot for the period")
	ErrMissingAttdDays     = errors.New("run: MISSING_ATTD_DAYS: pyr_attd_days has no row for the period")
	ErrMissingAttendance   = errors.New("run: MISSING_ATTENDANCE: active employee without attendance recap")
	ErrBadComponentCode    = errors.New("run: component code is not T/P + number")
	ErrBadSnapshotValue    = errors.New("run: attendance snapshot value is not a number")
	ErrBadPeriod           = errors.New("run: invalid period")
	ErrSubsidyBasisPeriods = errors.New("run: subsidy rules of the run use different basis periods")
	errUnknownComponentTyp = errors.New("run: unknown component type")
)

// Issue codes reported as warnings.
const (
	CodeMissingAttendance = "MISSING_ATTENDANCE"

	divJakarta = "JAKARTA"
	stsFull    = "F"
	// Legacy COMPNT_EMP subqueries of PPh ignore FRZ_FLAG for P4/P8 and P14/P18/P22/P23 (D-28d),
	// so a frozen row still reduces the gross / annual deduction (loadComponents).
	idAbsen           = 4
	idDeductionImpact = 8
	idGapok           = 1
	frzActive         = 2
)

// Issue is a validation finding of the load step (03§3 step 2).
type Issue struct {
	NIK     string
	Code    string
	Message string
}

// Query selects what to load. NIKs empty = every active employee of the division.
type Query struct {
	CompanyID int16
	Division  string
	YYMM      int
	NIKs      []string
}

// Rules is the part of the T016a rule set loader output that every Input shares.
// The loader adds YTD, TaxAdj and the per-employee subsidy basis (T016c); THR stays empty (B-THR).
type Rules struct {
	Set     engine.RuleSet
	Tax     engine.TaxTables
	Subsidy func(nik string) engine.Subsidy // nil = no subsidy input
}

// Result is the loaded batch plus its warnings.
type Result struct {
	Inputs   []engine.Input
	Warnings []Issue
}

// Loader reads the run input with a fixed number of set-based queries per division.
type Loader struct{}

// NewLoader returns a Loader.
func NewLoader() Loader { return Loader{} }

// Load builds one engine.Input per active employee (sts_payroll = 2), ordered by NIK.
// tx must be a scoped transaction (RLS), see postgres.DB.ScopedTx.
func (l Loader) Load(ctx context.Context, tx *sql.Tx, q Query, rules Rules) (Result, error) {
	period, err := periodOf(q.YYMM)
	if err != nil {
		return Result{}, err
	}
	workdays, err := loadWorkdays(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	snap, err := loadSnapshot(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	emps, err := loadEmployees(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	comps, err := loadComponents(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	adds, err := loadAdditions(ctx, tx, q)
	if err != nil {
		return Result{}, err
	}
	basisPeriod, err := subsidyBasisPeriod(rules)
	if err != nil {
		return Result{}, err
	}
	tax, taxIssues, err := loadTax(ctx, tx, q, rules.Set.LegacyCompat(), basisPeriod)
	if err != nil {
		return Result{}, err
	}

	var t1 map[string]bool
	if strings.EqualFold(q.Division, divJakarta) {
		if t1, err = loadGapokHolders(ctx, tx, q); err != nil {
			return Result{}, err
		}
	}

	res := Result{Warnings: taxIssues}
	for _, e := range emps {
		in := engine.Input{
			Period: period, Division: q.Division, Emp: e, Components: comps[e.NIK],
			Workdays: workdays, Additions: adds[e.NIK], Tax: rules.Tax, Rules: rules.Set,
		}
		in.Emp.GapokSts = gapokSts(in.Components)
		if rules.Subsidy != nil {
			in.Subsidy = rules.Subsidy(e.NIK)
		}
		tax[e.NIK].apply(&in)
		row, ok := snap[e.NIK]
		if !ok {
			if !rules.Set.LegacyCompat() {
				return Result{}, fmt.Errorf("%w: %s", ErrMissingAttendance, e.NIK)
			}
			res.Warnings = append(res.Warnings, Issue{NIK: e.NIK, Code: CodeMissingAttendance,
				Message: "karyawan aktif tanpa rekap absensi; header dilewati (D2)"})
		} else {
			in.Att, in.AttAdd, in.Sch = row.att, row.add, row.sch
			in.Daily = dailyFails(q.Division, row, t1[e.NIK])
		}
		res.Inputs = append(res.Inputs, in)
	}
	return res, nil
}

// subsidyBasisPeriod is the rule v1 P0 basis period (psr_basis_period_yymm, 202601 for the migrated
// rules). The per-employee basis is read once, so the active rules must agree on one period.
func subsidyBasisPeriod(rules Rules) (int, error) {
	if rules.Subsidy == nil || !rules.Set.LegacyCompat() {
		return 0, nil
	}
	period := 0
	for _, r := range rules.Subsidy("").Rules {
		if r.BasisPeriod == 0 || r.BasisPeriod == period {
			continue
		}
		if period != 0 {
			return 0, fmt.Errorf("%w: %d and %d", ErrSubsidyBasisPeriods, period, r.BasisPeriod)
		}
		period = r.BasisPeriod
	}
	return period, nil
}

// periodOf returns the period of yymm: the 26th of the previous month to the 25th (UTC dates).
func periodOf(yymm int) (engine.Period, error) {
	y, m := yymm/100, yymm%100
	if y < 1900 || m < 1 || m > 12 {
		return engine.Period{}, fmt.Errorf("%w: yymm %d", ErrBadPeriod, yymm)
	}
	end := time.Date(y, time.Month(m), 25, 0, 0, 0, 0, time.UTC)
	return engine.Period{YYMM: yymm, Start: end.AddDate(0, -1, 1), End: end}, nil
}

// gapokSts is the STS of T1 ("" when the employee has none).
func gapokSts(cs []engine.EmpComponent) string {
	for _, c := range cs {
		if c.Key == (engine.CompKey{Type: engine.CompT, ID: idGapok}) {
			return c.Sts
		}
	}
	return ""
}

// dailyFails picks the PREMI/BERAS fail counts of an employee (05§7). JAKARTA has no daily
// data: beras = alpa + ijin of the recap, only when the employee has T1 (legacy, no frz
// condition). The engine counts whole days, so a fraction is truncated.
func dailyFails(division string, r snapRow, hasT1 bool) engine.DailyFails {
	d := engine.DailyFails{PremiFail: r.premiFail, BerasFail: r.berasFail}
	if strings.EqualFold(division, divJakarta) {
		d.BerasFail = 0
		if hasT1 {
			d.BerasFail = int(r.att.Get("alpa").Add(r.att.Get("ijin")).IntPart())
		}
	}
	return d
}

// typeOf maps pyr_component.pc_type to the engine type.
func typeOf(s string) (engine.CompType, error) {
	switch strings.ToLower(s) {
	case "tunjangan", "t":
		return engine.CompT, nil
	case "potongan", "p":
		return engine.CompP, nil
	}
	return "", fmt.Errorf("%w: %q", errUnknownComponentTyp, s)
}
