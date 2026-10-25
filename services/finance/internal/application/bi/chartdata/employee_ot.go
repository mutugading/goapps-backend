package chartdata

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// tableModeEmployeeOT is the chart_config.table_mode of the "Employee Overtime" table (HD-18):
// every employee with OT in the R1 period window or limit hours in the R2 week, one row each.
const tableModeEmployeeOT = "employee_ot"

// Column keys of the Employee Overtime table besides colExcess/colChronic.
const (
	colPeriodHours = "PERIOD_HOURS" // R1: actual OT hours over the payroll periods of the window
	colPrevHours   = "PREV_HOURS"   // comparison: OT hours over the previous window of equal length (HD-24)
	colWeekHours   = "WEEK_HOURS"   // R2: limit hours of the R2 week
	colDayActual   = "ACTUAL"       // drill: actual OT hours per day (HR_OT_HOURS DAILY)
	colDayLimit    = "LIMIT"        // drill: limit hours per day of the R2 week (HR_OT_WEEK_DAILY)
)

// Group-by keys of the Employee Overtime table (HD-18, HD-19).
const (
	GroupByDivision = "DIVISION"
	GroupByDept     = "DEPT"
	GroupByCompany  = "COMPANY"
)

// Fact types read by the Employee Overtime table besides the employee_week ones.
const (
	TypeHoursName = "HR_OT_HOURS"
	TypeWeekName  = "HR_OT_WEEK"
)

// Marker rows of the previous-window total (HD-24). The total covers every employee with OT hours in
// the comparison window (group filter applied), not only the listed rows, so the KPI delta also sees
// employees who stopped working OT. They are not employees: shapeEmployeeOT splits them off first.
const (
	prevTotalCategory     = "\x1ePREV_TOTAL"
	colPrevTotalHours     = "PREV_TOTAL_HOURS"
	colPrevTotalEmployees = "PREV_TOTAL_EMPLOYEES"
)

// employeeOTConfigKey is the Config key of the Employee Overtime response block.
const employeeOTConfigKey = "employee_ot"

// employeeOTMaxRows caps the employee rows (HD-18); one more is fetched to detect truncation.
const employeeOTMaxRows = 2000

// employeeOTMaxGroupFilter caps the group filter values bound into one query.
const employeeOTMaxGroupFilter = 500

// employeeOTGroupExpr maps a group-by key to a fixed SQL expression over bi_hr_employee e. Only
// these constants reach the SQL text; the key itself is never interpolated.
var employeeOTGroupExpr = map[string]string{
	"":              "''",
	GroupByDivision: "COALESCE(e.division, '')",
	GroupByDept:     "COALESCE(e.dept_name, '')",
	GroupByCompany:  "COALESCE(e.company, '')",
}

// employeeOTGroupOptions lists the group-by keys always offered to the viewer. COMPANY is added
// only when a listed employee has a company in bi_hr_employee (HD-19, H026).
var employeeOTGroupOptions = []string{GroupByDivision, GroupByDept}

// EmployeeOTWindows are the two windows derived from one preset (HD-18).
type EmployeeOTWindows struct {
	PeriodFrom time.Time // first fact date (25th) of the R1 window, inclusive
	PeriodTo   time.Time // last fact date (25th) of the R1 window = end of the last period
	FirstStart time.Time // 26th that opens the first payroll period (header label)
	LastStart  time.Time // 26th that opens the last payroll period (daily drill)
	Week       time.Time // Monday of the R2 week: its Sunday falls in the last period (K10)
	DayFrom    time.Time // first day of the NIK→day drill: min(LastStart, Week)
	PrevFrom   time.Time // first fact date (25th) of the comparison window (HD-24)
	PrevTo     time.Time // last fact date (25th) of the comparison window = the period before PeriodFrom
}

// ResolveEmployeeOTWindows resolves the R1 period window and the R2 week for a preset. today is
// the WIB date. Every preset is anchored on the running payroll period (the one containing today)
// and counts payroll periods by the month of their closing 25th (label YYYYMM, K3): THIS_MONTH =
// running period, LAST_MONTH = the one before, THIS_QTR / THIS_YEAR = periods whose 25th is in the
// quarter / year of the running period, L12M = the last 12 periods. The R2 week is the latest week
// whose Sunday is inside the last period and whose Monday is not after today, so it may be partial
// (HD-17). DayFrom starts the NIK→day drill at whichever comes first: the last period or the week.
// The comparison window (HD-24) has exactly as many periods as the R1 window and ends right before it.
func ResolveEmployeeOTWindows(preset string, today time.Time) (EmployeeOTWindows, error) {
	last := payrollEnd(today)
	var first time.Time
	switch preset {
	case "THIS_MONTH":
		first = last
	case "LAST_MONTH":
		last = last.AddDate(0, -1, 0)
		first = last
	case "THIS_QTR":
		first = time.Date(last.Year(), ((last.Month()-1)/3)*3+1, 25, 0, 0, 0, 0, last.Location())
	case "THIS_YEAR":
		first = time.Date(last.Year(), time.January, 25, 0, 0, 0, 0, last.Location())
	case "L12M":
		first = last.AddDate(0, -11, 0)
	default:
		return EmployeeOTWindows{}, fmt.Errorf("%w: preset %q not allowed for employee_ot (period presets only)", factmetric.ErrInvalidPlan, preset)
	}
	lastStart := payrollStart(last)
	sunday := last.AddDate(0, 0, -int(last.Weekday())) // latest Sunday <= last (Sunday = 0)
	week := sunday.AddDate(0, 0, -6)
	if m := mondayOf(today); m.Before(week) {
		week = m
	}
	dayFrom := lastStart
	if week.Before(dayFrom) {
		dayFrom = week
	}
	periods := (last.Year()-first.Year())*12 + int(last.Month()) - int(first.Month()) + 1
	return EmployeeOTWindows{PeriodFrom: first, PeriodTo: last, FirstStart: payrollStart(first), LastStart: lastStart,
		Week: week, DayFrom: dayFrom, PrevFrom: first.AddDate(0, -periods, 0), PrevTo: first.AddDate(0, -1, 0)}, nil
}

// employeeOTPreset is the viewer preset, or the dashboard default when none is sent.
func employeeOTPreset(d *dashboarddomain.Dashboard, f ViewerFilters) string {
	if f.PeriodPreset == "" {
		return d.DefaultPeriod().String()
	}
	return f.PeriodPreset
}

// normalizeEmployeeOTGroupBy validates the group-by key against the whitelist (HD-18).
func normalizeEmployeeOTGroupBy(g string) (string, error) {
	k := strings.ToUpper(strings.TrimSpace(g))
	if _, ok := employeeOTGroupExpr[k]; ok {
		return k, nil
	}
	return "", fmt.Errorf("%w: group_by %q not allowed", factmetric.ErrInvalidPlan, g)
}

// planEmployeeOT builds the Employee Overtime table (HD-18) from bi_fact_metric: the union of
// NIKs with HR_OT_HOURS in the R1 window or HR_OT_WEEK > 0 in the R2 week, one row per
// (NIK, column). Group attributes come from bi_hr_employee at query time (HD-15). With
// DrillPath = [nik] it returns that employee's daily actual hours of the last period plus the
// limit hours of the R2 week. Every value is a bound arg; only constants are concatenated.
// Access is gated by RequestsEmployeeLevel (isEmployeeTable) before the plan is built.
func planEmployeeOT(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) (factmetric.PlannedQuery, error) {
	if n := len(f.DrillPath); n > d.MaxDrillLevel().Int() {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: depth %d > max %d", factmetric.ErrDrillTooDeep, n, d.MaxDrillLevel().Int())
	}
	w, err := ResolveEmployeeOTWindows(employeeOTPreset(d, f), wibDate(now))
	if err != nil {
		return factmetric.PlannedQuery{}, err
	}
	if len(f.DrillPath) > 0 {
		return planEmployeeOTDays(f.DrillPath[0], w), nil
	}
	groupBy, err := normalizeEmployeeOTGroupBy(f.GroupBy)
	if err != nil {
		return factmetric.PlannedQuery{}, err
	}
	if len(f.GroupFilter) > 0 && groupBy == "" {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: group filter needs a group_by", factmetric.ErrInvalidPlan)
	}
	if len(f.GroupFilter) > employeeOTMaxGroupFilter {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: too many group filter values", factmetric.ErrInvalidPlan)
	}
	expr := employeeOTGroupExpr[groupBy]
	args := []any{TypeHoursName, w.PeriodFrom, w.PeriodTo, TypeWeekName, TypeWeekExcessName, TypeChronicName, w.Week, employeeOTMaxRows + 1, w.PrevFrom, w.PrevTo}
	groupCond := ""
	if len(f.GroupFilter) > 0 {
		var conds []string
		conds, args, _ = appendINClause(nil, args, len(args)+1, expr, f.GroupFilter)
		groupCond = "\n    AND " + conds[0]
	}
	sql := `WITH a AS (
  SELECT group_3 AS nik, '` + colPeriodHours + `' AS col, SUM(value) AS v
  FROM bi_fact_metric
  WHERE type = $1 AND periode_grain = 'MONTHLY' AND is_active = TRUE
    AND periode_date BETWEEN $2 AND $3 AND COALESCE(group_3, '') <> ''
  GROUP BY group_3
  UNION ALL
  SELECT group_3, '` + colPrevHours + `', SUM(value)
  FROM bi_fact_metric
  WHERE type = $1 AND periode_grain = 'MONTHLY' AND is_active = TRUE
    AND periode_date BETWEEN $9 AND $10 AND COALESCE(group_3, '') <> ''
  GROUP BY group_3
  UNION ALL
  SELECT group_3, CASE type WHEN $4 THEN '` + colWeekHours + `' WHEN $5 THEN '` + colExcess + `' ELSE '` + colChronic + `' END,
         CASE WHEN type = $6 THEN MAX(value) ELSE SUM(value) END
  FROM bi_fact_metric
  WHERE type IN ($4, $5, $6) AND periode_grain = 'WEEKLY' AND is_active = TRUE
    AND periode_date = $7 AND COALESCE(group_3, '') <> ''
  GROUP BY group_3, type
), p AS (
  SELECT a.v FROM a LEFT JOIN bi_hr_employee e ON e.nik = a.nik
  WHERE a.col = '` + colPrevHours + `' AND a.v > 0` + groupCond + `
), n AS (
  SELECT a.nik FROM a LEFT JOIN bi_hr_employee e ON e.nik = a.nik
  WHERE (a.col = '` + colPeriodHours + `' OR (a.col = '` + colWeekHours + `' AND a.v > 0))` + groupCond + `
  GROUP BY a.nik
  ORDER BY SUM(CASE WHEN a.col = '` + colPeriodHours + `' THEN a.v ELSE 0 END) DESC,
           SUM(CASE WHEN a.col = '` + colWeekHours + `' THEN a.v ELSE 0 END) DESC, a.nik COLLATE "C"
  LIMIT $8
)
SELECT a.nik || chr(31) || ` + expr + ` || chr(31) || COALESCE(e.emp_name, '') || chr(31) || COALESCE(e.division, '') || chr(31) || COALESCE(e.dept_name, '') || chr(31) || COALESCE(e.company, '') AS category,
       NULL::date AS periode_date, a.col AS periode_label, a.v AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM a JOIN n ON n.nik = a.nik
LEFT JOIN bi_hr_employee e ON e.nik = a.nik
UNION ALL
SELECT '` + prevTotalCategory + `', NULL::date, '` + colPrevTotalHours + `', COALESCE(SUM(v), 0), 0::numeric, 0::int FROM p
UNION ALL
SELECT '` + prevTotalCategory + `', NULL::date, '` + colPrevTotalEmployees + `', COUNT(*)::numeric, 0::numeric, 0::int FROM p`
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}, nil
}

// planEmployeeOTDays is the NIK→day drill: actual OT per day from DayFrom to the end of the last
// payroll period (ACTUAL; covers the R2 week days before the 26th too) and limit hours per day of
// the R2 week (LIMIT).
func planEmployeeOTDays(nik string, w EmployeeOTWindows) factmetric.PlannedQuery {
	args := []any{nik, TypeHoursName, w.DayFrom, w.PeriodTo, TypeWeekDailyName, w.Week, w.Week.AddDate(0, 0, 6)}
	sql := `SELECT '` + colDayActual + `'::text AS category, periode_date, TO_CHAR(periode_date, 'YYYY-MM-DD')::text AS periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM bi_fact_metric
WHERE type = $2 AND periode_grain = 'DAILY' AND is_active = TRUE AND group_3 = $1
  AND periode_date BETWEEN $3 AND $4
GROUP BY periode_date
UNION ALL
SELECT '` + colDayLimit + `'::text, periode_date, TO_CHAR(periode_date, 'YYYY-MM-DD')::text,
       SUM(value), 0::numeric, 0::int
FROM bi_fact_metric
WHERE type = $5 AND periode_grain = 'DAILY' AND is_active = TRUE AND group_3 = $1
  AND periode_date BETWEEN $6 AND $7
GROUP BY periode_date
ORDER BY 2, 1`
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}
}

// employeeOTRow is one employee of the table with its group and column values.
type employeeOTRow struct {
	nik, group, name, division, dept, company string
	vals                                      map[string]float64
}

// meta is the point meta of the employee: identity plus the comparison (HD-24). prev_hours and
// delta_pct are nil when the employee has no hours in the previous window (shown as "–").
func (e *employeeOTRow) meta() map[string]any {
	m := map[string]any{"name": e.name, "group": e.group, "division": e.division, "dept": e.dept, "company": e.company,
		"prev_hours": nil, "delta_pct": nil}
	if prev, ok := e.vals[colPrevHours]; ok {
		m["prev_hours"] = prev
		m["delta_pct"] = pctDelta(e.vals[colPeriodHours], prev)
	}
	return m
}

// employeeOTColumns are the table columns in display order.
var employeeOTColumns = []string{colPeriodHours, colWeekHours, colExcess, colChronic}

// collectEmployeeOT folds (nik|group|name|division|dept|company, column, value) rows into employees.
func collectEmployeeOT(rows []factmetric.AggRow) []*employeeOTRow {
	byNIK := map[string]*employeeOTRow{}
	out := make([]*employeeOTRow, 0)
	for _, r := range rows {
		parts := strings.SplitN(r.Category, empSep, 6)
		e := byNIK[parts[0]]
		if e == nil {
			e = &employeeOTRow{nik: parts[0], vals: map[string]float64{}}
			if len(parts) == 6 {
				e.group, e.name, e.division, e.dept, e.company = parts[1], parts[2], parts[3], parts[4], parts[5]
			}
			byNIK[parts[0]] = e
			out = append(out, e)
		}
		e.vals[r.PeriodLabel] += r.Value
	}
	return out
}

// sortEmployeeOT orders rows by group, then rank (rankBefore).
func sortEmployeeOT(emps []*employeeOTRow) {
	sort.Slice(emps, func(i, j int) bool {
		if emps[i].group != emps[j].group {
			return emps[i].group < emps[j].group
		}
		return rankBefore(emps[i], emps[j])
	})
}

// employeeOTSubtotals sums the hour columns per group (CHRONIC is a streak, never summed).
func employeeOTSubtotals(emps []*employeeOTRow) []any {
	type subtotal struct {
		group string
		n     float64
		over  float64
		sums  map[string]float64
	}
	var order []*subtotal
	byGroup := map[string]*subtotal{}
	for _, e := range emps {
		s := byGroup[e.group]
		if s == nil {
			s = &subtotal{group: e.group, sums: map[string]float64{}}
			byGroup[e.group] = s
			order = append(order, s)
		}
		s.n++
		if e.vals[colExcess] > 0 {
			s.over++
		}
		for _, c := range []string{colPeriodHours, colWeekHours, colExcess} {
			s.sums[c] += e.vals[c]
		}
	}
	out := make([]any, 0, len(order))
	for _, s := range order {
		out = append(out, map[string]any{"group": s.group, "employees": s.n,
			colPeriodHours: s.sums[colPeriodHours], colWeekHours: s.sums[colWeekHours], colExcess: s.sums[colExcess],
			"avg_period_hours": round1(s.sums[colPeriodHours] / s.n), "over_pct": round1(s.over / s.n * 100)})
	}
	return out
}

// round1 rounds to one decimal, half away from zero (= half-up for the non-negative values here, D-07).
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// pctDelta is the change from prev to cur in percent, 1 decimal. nil when there is nothing to compare
// against (prev <= 0), so the UI shows "–" instead of a misleading 0% or infinity.
func pctDelta(cur, prev float64) any {
	if prev <= 0 {
		return nil
	}
	return round1((cur - prev) / prev * 100)
}

// prevTotals are the previous-window totals read from the marker rows.
type prevTotals struct{ hours, employees float64 }

// splitPrevTotals separates the marker rows from the employee rows. totals is nil when the query
// returned no marker rows or no employee had hours in the previous window.
func splitPrevTotals(rows []factmetric.AggRow) ([]factmetric.AggRow, *prevTotals) {
	var t prevTotals
	kept := make([]factmetric.AggRow, 0, len(rows))
	for _, r := range rows {
		if r.Category != prevTotalCategory {
			kept = append(kept, r)
			continue
		}
		switch r.PeriodLabel {
		case colPrevTotalHours:
			t.hours = r.Value
		case colPrevTotalEmployees:
			t.employees = r.Value
		}
	}
	if t.employees <= 0 {
		return kept, nil
	}
	return kept, &t
}

// employeeOTPrev is the comparison of the headline (HD-24): current listed hours and employees against
// the full previous-window totals. nil (shown as "–") when the previous window has no OT.
func employeeOTPrev(prev *prevTotals, hours, n float64) any {
	if prev == nil {
		return nil
	}
	return map[string]any{"total_period_hours": round1(prev.hours), "employees": prev.employees,
		"hours_delta_pct": pctDelta(hours, prev.hours), "employees_delta_pct": pctDelta(n, prev.employees)}
}

// employeeOTSummary is the headline of the table (HD-24) over the same rows as the subtotals:
// employees listed, total period hours, employees over the limit (EXCESS > 0) and chronic ones
// (CHRONIC > 0, counted per person, never summed, HD-16).
func employeeOTSummary(emps []*employeeOTRow, prev *prevTotals) map[string]any {
	var hours, over, chronic float64
	for _, e := range emps {
		hours += e.vals[colPeriodHours]
		if e.vals[colExcess] > 0 {
			over++
		}
		if e.vals[colChronic] > 0 {
			chronic++
		}
	}
	return map[string]any{"employees": float64(len(emps)), "total_period_hours": round1(hours),
		"over_limit": over, "chronic": chronic, "prev": employeeOTPrev(prev, hours, float64(len(emps)))}
}

// shapeEmployeeOT turns planEmployeeOT rows into one Series per column over NIK categories,
// truncated to employeeOTMaxRows, and the "employee_ot" block of the response config: group-by,
// group options, per-group subtotals and the truncated flag. Missing columns are 0. When truncated,
// subtotals (and the filter values the FE derives from the rows) cover the listed rows only.
func shapeEmployeeOT(rows []factmetric.AggRow, f ViewerFilters, numFmt chart.NumberFormat, decimals int) ([]Series, []string, map[string]any) {
	rows, prev := splitPrevTotals(rows)
	emps := collectEmployeeOT(rows)
	truncated := len(emps) > employeeOTMaxRows
	if truncated {
		// The SQL keeps the top employeeOTMaxRows+1 in rank order; drop the lowest-ranked one.
		sort.Slice(emps, func(i, j int) bool { return rankBefore(emps[i], emps[j]) })
		emps = emps[:employeeOTMaxRows]
	}
	sortEmployeeOT(emps)
	niks := make([]string, 0, len(emps))
	for _, e := range emps {
		niks = append(niks, e.nik)
	}
	series := make([]Series, 0, len(employeeOTColumns))
	for _, c := range employeeOTColumns {
		cf, cd := numFmt, decimals
		if c == colChronic {
			cf, cd = chart.NumberFormat("raw"), 0
		}
		pts := make([]DataPoint, 0, len(emps))
		for _, e := range emps {
			v := e.vals[c]
			pts = append(pts, DataPoint{Category: e.nik, Value: v, Label: chart.Format(v, cf, cd), Meta: e.meta()})
		}
		series = append(series, Series{Name: c, Points: pts})
	}
	groupBy, _ := normalizeEmployeeOTGroupBy(f.GroupBy) //nolint:errcheck // validated by the planner
	opts := make([]any, 0, len(employeeOTGroupOptions)+1)
	for _, o := range employeeOTGroupOptions {
		opts = append(opts, o)
	}
	for _, e := range emps {
		if e.company != "" {
			opts = append(opts, GroupByCompany)
			break
		}
	}
	block := map[string]any{
		"group_by":      groupBy,
		"group_options": opts,
		"truncated":     truncated,
		"max_rows":      float64(employeeOTMaxRows),
		"subtotals":     employeeOTSubtotals(emps),
		"summary":       employeeOTSummary(emps, prev),
	}
	return series, niks, block
}

// rankBefore is the SQL row-limit order: period hours, week hours (both descending), then NIK
// byte order (COLLATE "C" in SQL), so the dropped extra row is the one SQL ranked last.
func rankBefore(a, b *employeeOTRow) bool {
	if a.vals[colPeriodHours] != b.vals[colPeriodHours] {
		return a.vals[colPeriodHours] > b.vals[colPeriodHours]
	}
	if a.vals[colWeekHours] != b.vals[colWeekHours] {
		return a.vals[colWeekHours] > b.vals[colWeekHours]
	}
	return a.nik < b.nik
}

// shapeEmployeeOTDays turns the drill rows into two Series (ACTUAL, LIMIT) over the union of
// days, ascending; a day missing from one series is 0.
func shapeEmployeeOTDays(rows []factmetric.AggRow, numFmt chart.NumberFormat, decimals int) ([]Series, []string) {
	vals := map[string]map[string]float64{colDayActual: {}, colDayLimit: {}}
	seen := map[string]struct{}{}
	days := make([]string, 0)
	for _, r := range rows {
		if vals[r.Category] == nil {
			continue
		}
		vals[r.Category][r.PeriodLabel] += r.Value
		if _, ok := seen[r.PeriodLabel]; !ok {
			seen[r.PeriodLabel] = struct{}{}
			days = append(days, r.PeriodLabel)
		}
	}
	sort.Strings(days)
	series := make([]Series, 0, 2)
	for _, c := range []string{colDayActual, colDayLimit} {
		pts := make([]DataPoint, 0, len(days))
		for _, d := range days {
			v := vals[c][d]
			pts = append(pts, DataPoint{Category: d, Value: v, Label: chart.Format(v, numFmt, decimals)})
		}
		series = append(series, Series{Name: c, Points: pts})
	}
	return series, days
}

// shapeEmployeeOTResult builds the viewer payload of the Employee Overtime dashboard. The table
// view adds an "employee_ot" block to Config (group options, subtotals, truncated); the range
// labels are added by addEmployeeOTRanges with the same anchor the plan used.
func shapeEmployeeOTResult(d *dashboarddomain.Dashboard, rows []factmetric.AggRow, kpis []KpiResult, f ViewerFilters,
	drillContext DrillContext, numFmt chart.NumberFormat, decimals int) Result {
	cfg := d.ChartConfig().MarshalToMap()
	var series []Series
	var cats []string
	if len(f.DrillPath) > 0 {
		series, cats = shapeEmployeeOTDays(rows, numFmt, decimals)
	} else {
		var block map[string]any
		series, cats, block = shapeEmployeeOT(rows, f, numFmt, decimals)
		if cfg == nil {
			cfg = map[string]any{}
		}
		cfg[employeeOTConfigKey] = block
	}
	return Result{
		Config:       cfg,
		Series:       series,
		Categories:   cats,
		KPIs:         kpis,
		DrillContext: drillContext,
		Meta:         Meta{AsOf: time.Now().UTC(), RowCount: len(rows)},
	}
}

// addEmployeeOTRanges puts the column range labels (R1 periods, last period, R2 week) into the
// employee_ot block, resolved at the same anchor `now` the plan used. No-op for other dashboards
// and for the day drill. The preset was already validated by Plan, so an invalid one only leaves
// the ranges out.
func addEmployeeOTRanges(r *Result, d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) {
	if !isEmployeeOT(d) || len(f.DrillPath) > 0 {
		return
	}
	block, ok := r.Config[employeeOTConfigKey].(map[string]any)
	if !ok {
		return
	}
	w, err := ResolveEmployeeOTWindows(employeeOTPreset(d, f), wibDate(now))
	if err != nil {
		return
	}
	block["ranges"] = map[string]any{
		"period_from": w.FirstStart.Format(time.DateOnly), "period_to": w.PeriodTo.Format(time.DateOnly),
		"last_period_from": w.LastStart.Format(time.DateOnly), "day_from": w.DayFrom.Format(time.DateOnly),
		"week_from": w.Week.Format(time.DateOnly), "week_to": w.Week.AddDate(0, 0, 6).Format(time.DateOnly),
		"week_label": PeriodLabel(w.Week, grainWeekly),
		"prev_from":  payrollStart(w.PrevFrom).Format(time.DateOnly), "prev_to": w.PrevTo.Format(time.DateOnly),
	}
}
