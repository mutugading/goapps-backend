package chartdata_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
)

func employeeOTCfg() map[string]any {
	return map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "table_mode": "employee_ot"}
}

func employeeOTDashboard(t *testing.T) *dashboarddomain.Dashboard {
	t.Helper()
	return newPivotDashboard(t, "data_table", "WEEKLY", "", employeeOTCfg())
}

func isoDay(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// otNow is 2026-10-21 10:00 WIB (Wednesday), inside payroll period 26 Sep – 25 Oct.
var otNow = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)

func TestEmployeeOT_GatedWithoutPermission(t *testing.T) {
	dash := employeeOTDashboard(t)
	repo := &mockDashboardRepo{getByCode: func(context.Context, string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	fact := &mockFactRepo{}
	h := chartdata.NewGetDataHandler(repo, fact, newMockCache(), func(any) string { return "h" }).WithNow(func() time.Time { return otNow })
	_, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "HR_OT_EMPLOYEES", CanViewWorkforce: true,
		Filters: chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT"},
	})
	if !errors.Is(err, dashboarddomain.ErrForbidden) || fact.queries != 0 {
		t.Fatalf("want forbidden without query, err=%v queries=%d", err, fact.queries)
	}
	res, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "HR_OT_EMPLOYEES", CanViewWorkforce: true, CanViewEmployee: true,
		Filters: chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT"},
	})
	if err != nil {
		t.Fatalf("with permission: %v", err)
	}
	// Range labels follow the handler clock (WIB today = Wed 21 Oct 2026), not time.Now.
	ranges := res.Config["employee_ot"].(map[string]any)["ranges"].(map[string]any)
	if ranges["period_from"] != "2026-09-26" || ranges["period_to"] != "2026-10-25" ||
		ranges["week_from"] != "2026-10-19" || ranges["week_to"] != "2026-10-25" || ranges["week_label"] != "2026-W43" {
		t.Errorf("ranges = %v", ranges)
	}
}

func TestEmployeeOT_GroupByWhitelist(t *testing.T) {
	d := employeeOTDashboard(t)
	for _, g := range []string{"", "DIVISION", "dept", " Dept "} {
		if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: g}, otNow); err != nil {
			t.Errorf("group_by %q: %v", g, err)
		}
	}
	for _, g := range []string{"COMPANY", "group_1", "e.nik", "DEPT; DROP TABLE bi_fact_metric--", "1) OR (1=1"} {
		_, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: g}, otNow)
		if !errors.Is(err, factmetric.ErrInvalidPlan) {
			t.Errorf("group_by %q: want ErrInvalidPlan, got %v", g, err)
		}
	}
}

func TestEmployeeOT_GroupFilterIsBound(t *testing.T) {
	d := employeeOTDashboard(t)
	evil := "PRD') OR ('1'='1"
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT", GroupFilter: []string{evil, "QA"}}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.SQL, evil) || !strings.Contains(p.SQL, "COALESCE(e.dept_name, '') IN ($9,$10)") {
		t.Errorf("filter not bound:\n%s", p.SQL)
	}
	if p.Args[8] != evil || p.Args[9] != "QA" {
		t.Errorf("args = %v", p.Args)
	}
	many := make([]string, 501)
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT", GroupFilter: many}, otNow); !errors.Is(err, factmetric.ErrInvalidPlan) {
		t.Errorf("too many filter values: %v", err)
	}
}

func TestEmployeeOT_CacheHashDiffersPerGroup(t *testing.T) {
	base := chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}
	byDept, byDiv := base, base
	byDept.GroupBy, byDiv.GroupBy = "DEPT", "DIVISION"
	filtered := byDept
	filtered.GroupFilter = []string{"PRD"}
	hashes := map[string]bool{}
	for _, f := range []chartdata.ViewerFilters{base, byDept, byDiv, filtered} {
		hashes[redisinfra.HashFilters(f)] = true
	}
	if len(hashes) != 4 {
		t.Fatalf("want 4 distinct hashes, got %d", len(hashes))
	}
}

func TestEmployeeOT_RejectsWeekAndDayPresets(t *testing.T) {
	d := employeeOTDashboard(t)
	for _, p := range []string{"THIS_WEEK", "LAST_WEEK", "YESTERDAY", "CUSTOM", "ALL", "L24M"} {
		if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: p}, otNow); !errors.Is(err, factmetric.ErrInvalidPlan) {
			t.Errorf("preset %s: want ErrInvalidPlan, got %v", p, err)
		}
	}
	// Empty preset falls back to the dashboard default (L12M here).
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{}, otNow); err != nil {
		t.Errorf("default preset: %v", err)
	}
}

func TestResolveEmployeeOTWindows(t *testing.T) {
	cases := []struct {
		name, preset, today              string
		from, to, lastStart, week, first string
	}{
		// Week 23–29 Nov 2026 crosses the 25th/26th; its Sunday (29 Nov) is in period 26 Nov – 25 Dec (K10).
		{"minggu lintas 25/26 di periode Desember", "THIS_MONTH", "2026-11-27", "2026-12-25", "2026-12-25", "2026-11-26", "2026-11-23", "2026-11-26"},
		// Period 26 Oct – 25 Nov: last Sunday on/before 25 Nov is 22 Nov → week 16–22 Nov.
		{"periode lalu: minggu sebelum lintas", "LAST_MONTH", "2026-11-27", "2026-11-25", "2026-11-25", "2026-10-26", "2026-11-16", "2026-10-26"},
		// Running week (HD-17): today Wed 21 Oct, week 19–25 Oct belongs to period ending 25 Oct.
		{"minggu berjalan parsial", "THIS_MONTH", "2026-10-21", "2026-10-25", "2026-10-25", "2026-09-26", "2026-10-19", "2026-09-26"},
		{"L12M", "L12M", "2026-10-21", "2025-11-25", "2026-10-25", "2026-09-26", "2026-10-19", "2025-10-26"},
		{"THIS_QTR memuat periode berjalan", "THIS_QTR", "2026-10-21", "2026-10-25", "2026-10-25", "2026-09-26", "2026-10-19", "2026-09-26"},
		{"THIS_YEAR", "THIS_YEAR", "2026-10-21", "2026-01-25", "2026-10-25", "2026-09-26", "2026-10-19", "2025-12-26"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, err := chartdata.ResolveEmployeeOTWindows(c.preset, isoDay(c.today))
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%s %s %s %s %s", w.PeriodFrom.Format(time.DateOnly), w.PeriodTo.Format(time.DateOnly),
				w.LastStart.Format(time.DateOnly), w.Week.Format(time.DateOnly), w.FirstStart.Format(time.DateOnly))
			want := strings.Join([]string{c.from, c.to, c.lastStart, c.week, c.first}, " ")
			if got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func TestEmployeeOT_PlanArgsCrossingWeek(t *testing.T) {
	d := employeeOTDashboard(t)
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Args[1] != isoDay("2026-12-25") || p.Args[2] != isoDay("2026-12-25") || p.Args[6] != isoDay("2026-11-23") {
		t.Errorf("args = %v", p.Args)
	}
	if p.Args[7] != chartdata.EmployeeOTMaxRows+1 || !strings.Contains(p.SQL, "LIMIT $8") {
		t.Errorf("row limit missing: %v", p.Args[7])
	}
}

func otRow(nik, group, col string, v float64) factmetric.AggRow {
	sep := "\x1f"
	return factmetric.AggRow{Category: nik + sep + group + sep + "Nama " + nik + sep + "DIV" + sep + group, PeriodLabel: col, Value: v}
}

func TestShapeEmployeeOT_UnionZeroColumnsAndSubtotals(t *testing.T) {
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 40), // R1 only: week columns 0
		otRow("N2", "PRD", "WEEK_HOURS", 16),   // R2 only: period hours 0
		otRow("N2", "PRD", "EXCESS", 2),
		otRow("N2", "PRD", "CHRONIC", 3),
		otRow("N3", "QA", "PERIOD_HOURS", 10),
		otRow("N3", "QA", "WEEK_HOURS", 5),
	}
	series, cats, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{GroupBy: "dept"})
	if strings.Join(cats, ",") != "N1,N2,N3" {
		t.Fatalf("cats = %v", cats)
	}
	if len(series) != 4 || series[0].Name != "PERIOD_HOURS" || series[3].Name != "CHRONIC" {
		t.Fatalf("series = %+v", series)
	}
	if series[0].Points[1].Value != 0 || series[1].Points[0].Value != 0 || series[3].Points[1].Value != 3 {
		t.Errorf("zero fill wrong: %+v", series)
	}
	if series[0].Points[0].Meta["group"] != "PRD" || series[0].Points[0].Meta["name"] != "Nama N1" {
		t.Errorf("meta = %v", series[0].Points[0].Meta)
	}
	if block["group_by"] != "DEPT" || block["truncated"] != false {
		t.Errorf("block = %v", block)
	}
	subs := block["subtotals"].([]any)
	prd := subs[0].(map[string]any)
	if len(subs) != 2 || prd["group"] != "PRD" || prd["employees"] != 2.0 || prd["PERIOD_HOURS"] != 40.0 || prd["WEEK_HOURS"] != 16.0 || prd["EXCESS"] != 2.0 {
		t.Errorf("subtotals = %v", subs)
	}
	if _, ok := prd["CHRONIC"]; ok {
		t.Error("CHRONIC must not be summed")
	}
	opts := block["group_options"].([]any)
	if len(opts) != 2 || opts[0] != "DIVISION" || opts[1] != "DEPT" {
		t.Errorf("group_options = %v (COMPANY must be absent, HD-19)", opts)
	}
}

func TestShapeEmployeeOT_Truncated(t *testing.T) {
	rows := make([]factmetric.AggRow, 0, chartdata.EmployeeOTMaxRows+1)
	for i := 0; i <= chartdata.EmployeeOTMaxRows; i++ {
		rows = append(rows, otRow(fmt.Sprintf("N%05d", i), "PRD", "PERIOD_HOURS", float64(i+1)))
	}
	_, cats, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	if len(cats) != chartdata.EmployeeOTMaxRows || block["truncated"] != true {
		t.Fatalf("len=%d truncated=%v", len(cats), block["truncated"])
	}
	for _, c := range cats {
		if c == "N00000" {
			t.Fatal("lowest-ranked employee must be dropped")
		}
	}
}

func TestEmployeeOT_DrillDays(t *testing.T) {
	d := employeeOTDashboard(t)
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", DrillPath: []string{"N1"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Args[0] != "N1" || p.Args[2] != isoDay("2026-11-26") || p.Args[3] != isoDay("2026-12-25") ||
		p.Args[5] != isoDay("2026-11-23") || p.Args[6] != isoDay("2026-11-29") || !strings.Contains(p.SQL, "'DAILY'") {
		t.Errorf("drill args = %v", p.Args)
	}
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", DrillPath: []string{"a", "b", "c", "d"}}, now); !errors.Is(err, factmetric.ErrDrillTooDeep) {
		t.Errorf("too deep: %v", err)
	}
	series, days := chartdata.ShapeEmployeeOTDaysForTest([]factmetric.AggRow{
		{Category: "ACTUAL", PeriodLabel: "2026-11-24", Value: 3},
		{Category: "ACTUAL", PeriodLabel: "2026-11-27", Value: 2},
		{Category: "LIMIT", PeriodLabel: "2026-11-24", Value: 3},
		{Category: "LIMIT", PeriodLabel: "2026-11-23", Value: 8},
	})
	if strings.Join(days, ",") != "2026-11-23,2026-11-24,2026-11-27" || len(series) != 2 {
		t.Fatalf("days = %v", days)
	}
	if series[0].Points[0].Value != 0 || series[1].Points[0].Value != 8 || series[1].Points[2].Value != 0 {
		t.Errorf("series = %+v", series)
	}
}
