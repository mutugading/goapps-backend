package chartdata

import (
	"fmt"
	"strings"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ViewerFilters carries the runtime filter state from the viewer page.
type ViewerFilters struct {
	PeriodPreset string    // L12M | L24M | THIS_YEAR | THIS_QTR | THIS_MONTH | ALL | CUSTOM
	PeriodFrom   time.Time // valid when CUSTOM
	PeriodTo     time.Time
	Compare      string   // none | MoM | QoQ | YoY | YTD | R12
	DrillPath    []string // depth 0 = aggregate, 1 = into group_2, 2 = into group_3
}

// Plan turns a Dashboard + ViewerFilters into a PlannedQuery against the right MV / fact table.
//
// Drill-depth rules:
//   - 0 → mv_bi_metric_g1 (or filtered by dashboard.filter_group_1)
//   - 1 → mv_bi_metric_g2
//   - 2 → bi_fact_metric (group_3 level, raw)
//   - >max_drill_level → ErrDrillTooDeep
//
// Compare modes produce an extra `prev_value` column via LEFT JOIN on shifted periods.
func Plan(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) (factmetric.PlannedQuery, error) {
	drillDepth := len(f.DrillPath)
	if drillDepth > d.MaxDrillLevel().Int() {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: depth %d > max %d", factmetric.ErrDrillTooDeep, drillDepth, d.MaxDrillLevel().Int())
	}

	period := ResolvePeriod(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, d.PeriodGrain().String(), now)

	switch drillDepth {
	case 0:
		return planLevel1(d, f, period)
	case 1:
		return planLevel2(d, f, period)
	case 2:
		return planLevel3(d, f, period)
	}
	return factmetric.PlannedQuery{}, fmt.Errorf("%w: drill depth %d not supported", factmetric.ErrInvalidPlan, drillDepth)
}

// planLevel1 — aggregate at group_1 within (or below) the dashboard's filter_type.
//
// When filter_group_1 is set, drilling at level 0 means "show group_2 breakdown for that fixed group_1",
// effectively treating the dashboard as already drilled one level. This matches the EBITDA pattern
// (filter_type=MIS, filter_group_1=EBITDA → waterfall shows G2 components).
func planLevel1(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange) (factmetric.PlannedQuery, error) {
	if d.FilterGroup1() != "" {
		// Dashboard pre-narrows to a specific group_1 → render its group_2 breakdown.
		return buildPlan(buildArgs{
			Source:     "mv_bi_metric_g2",
			CategoryCol: "group_2",
			OrderCol:    "group_2_order",
			Type:        d.FilterType(),
			Group1:      d.FilterGroup1(),
			Period:      period,
			Grain:       d.PeriodGrain().String(),
			Compare:     f.Compare,
		})
	}
	return buildPlan(buildArgs{
		Source:      "mv_bi_metric_g1",
		CategoryCol: "group_1",
		OrderCol:    "group_1_order",
		Type:        d.FilterType(),
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
	})
}

func planLevel2(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange) (factmetric.PlannedQuery, error) {
	// DrillPath[0] picks a group_2 (when dashboard had filter_group_1) or a group_1 (when it didn't).
	if d.FilterGroup1() != "" {
		// Level 2 with pre-narrow → into group_3 of (filter_group_1, drill[0]).
		return buildPlan(buildArgs{
			Source:      "bi_fact_metric",
			CategoryCol: "group_3",
			OrderCol:    "group_3_order",
			Type:        d.FilterType(),
			Group1:      d.FilterGroup1(),
			Group2:      f.DrillPath[0],
			Period:      period,
			Grain:       d.PeriodGrain().String(),
			Compare:     f.Compare,
			RequireG3:   true,
		})
	}
	return buildPlan(buildArgs{
		Source:      "mv_bi_metric_g2",
		CategoryCol: "group_2",
		OrderCol:    "group_2_order",
		Type:        d.FilterType(),
		Group1:      f.DrillPath[0],
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
	})
}

func planLevel3(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange) (factmetric.PlannedQuery, error) {
	// Without filter_group_1: drill[0]=group_1, drill[1]=group_2 → render group_3.
	// With filter_group_1:    drill[0]=group_2, drill[1]=group_3 → already at max depth (level3 isn't drillable).
	if d.FilterGroup1() != "" {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: cannot drill past group_3 when dashboard pre-filters group_1", factmetric.ErrInvalidPlan)
	}
	return buildPlan(buildArgs{
		Source:      "bi_fact_metric",
		CategoryCol: "group_3",
		OrderCol:    "group_3_order",
		Type:        d.FilterType(),
		Group1:      f.DrillPath[0],
		Group2:      f.DrillPath[1],
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
		RequireG3:   true,
	})
}

// buildArgs are the inputs to buildPlan.
type buildArgs struct {
	Source      string
	CategoryCol string
	OrderCol    string
	Type        string
	Group1      string
	Group2      string
	Period      PeriodRange
	Grain       string
	Scenario    string
	Compare     string
	RequireG3   bool // when true, add `AND group_3 IS NOT NULL`
}

// buildPlan emits the canonical 6-column SELECT used by BiFactMetricRepository.QueryAggregate.
//
// Columns (in order): category, period_date, period_label, value, prev_value, order_seq.
// For non-compare queries, prev_value is 0 (placeholder).
//
// For compare modes that overlay a comparison series, a LEFT JOIN to a prev-period CTE
// adds the prev_value column. The shift amount depends on compare+grain.
func buildPlan(a buildArgs) (factmetric.PlannedQuery, error) {
	if a.Scenario == "" {
		a.Scenario = "ACTUAL"
	}
	args := []any{a.Type}
	idx := 2
	var conds []string
	conds = append(conds, "type = $1")
	if a.Group1 != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, a.Group1)
		idx++
	}
	if a.Group2 != "" {
		conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, a.Group2)
		idx++
	}
	if a.RequireG3 {
		conds = append(conds, "group_3 IS NOT NULL")
	}
	conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
	args = append(args, a.Grain)
	idx++
	if !a.Period.From.IsZero() && !a.Period.To.IsZero() {
		conds = append(conds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, a.Period.From, a.Period.To)
		idx += 2
	}
	conds = append(conds, fmt.Sprintf("scenario = $%d", idx))
	args = append(args, a.Scenario)
	// idx++ omitted; final.

	where := strings.Join(conds, " AND ")

	// Group by category (and optionally period) — for trend charts we group by period;
	// for categorical waterfall/bar/donut, the planner is generally invoked once with
	// the full period range and the SQL aggregates over time.
	//
	// We emit two output shapes interleaved by ORDER BY:
	//   - If category is a period field, callers pass CategoryCol="periode_date" and we
	//     return one row per period.
	//   - Otherwise we sum across the period range, returning one row per distinct category.
	groupByPeriod := a.CategoryCol == "periode_date"

	var selectExpr, groupBy, orderBy string
	if groupByPeriod {
		selectExpr = "TO_CHAR(periode_date, 'YYYYMMDD')::text AS category, periode_date, periode_label, SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq"
		groupBy = "periode_date, periode_label"
		orderBy = "periode_date"
	} else {
		categoryExpr := "COALESCE(" + a.CategoryCol + ", '')"
		var orderExpr string
		if a.OrderCol != "" {
			orderExpr = "COALESCE(MAX(" + a.OrderCol + "), 0)"
		} else {
			orderExpr = "0"
		}
		selectExpr = fmt.Sprintf(
			"%s AS category, NULL::date AS periode_date, ''::text AS periode_label, SUM(value) AS value, 0::numeric AS prev_value, %s AS order_seq",
			categoryExpr, orderExpr)
		groupBy = a.CategoryCol
		orderBy = "order_seq NULLS LAST, category"
	}

	sql := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE %s
GROUP BY %s
ORDER BY %s`, selectExpr, a.Source, where, groupBy, orderBy)

	return factmetric.PlannedQuery{
		SQL:         sql,
		Args:        args,
		TargetTable: a.Source,
	}, nil
}
