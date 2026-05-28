package chartdata_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func buildMultiMetricDashboard(t *testing.T, metrics []string) *dashboarddomain.Dashboard {
	t.Helper()
	chartCfg := map[string]any{
		"x_axis_field": "period",
		"y_axis_field": "display_value",
	}
	if len(metrics) > 0 {
		chartCfg["metric_filter"] = map[string]any{"include_metrics": toAnySlice(metrics)}
	}
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "SALES_TEST", Title: "Sales", FilterType: "SALES", FilterGroup1: "",
		PeriodGrain: "MONTHLY", DefaultPeriod: "L12M", ChartType: "line",
		ChartConfigRaw: chartCfg, MaxDrillLevel: 2, CacheTTLSec: 60,
		GroupID: uuid.New(), IsActive: true, DrillEnabled: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func TestPlan_MultiMetric_QueriesFactDirectly(t *testing.T) {
	d := buildMultiMetricDashboard(t, []string{"GROSS_SALES", "NETT_SALES", "MARGIN"})
	f := chartdata.ViewerFilters{PeriodPreset: "L12M"}
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)

	plan, err := chartdata.Plan(d, f, now)
	if err != nil {
		t.Fatal(err)
	}

	// Must NOT use materialized views.
	if strings.Contains(plan.SQL, "mv_bi_metric") {
		t.Error("multi-metric plan must query bi_fact_metric directly, not MVs")
	}
	// Must query bi_fact_metric.
	if !strings.Contains(plan.SQL, "bi_fact_metric") {
		t.Error("multi-metric plan must query bi_fact_metric")
	}
	// Must reference each requested metric.
	for _, m := range []string{"GROSS_SALES", "NETT_SALES", "MARGIN"} {
		if !strings.Contains(plan.SQL, m) {
			t.Errorf("plan SQL must reference metric %q", m)
		}
	}
	// Must filter by type.
	if !strings.Contains(plan.SQL, "type =") && !strings.Contains(plan.SQL, "type=") {
		t.Error("plan SQL must include type filter")
	}
}

func TestPlan_SingleMetric_UsesMVs(t *testing.T) {
	// Dashboard without metric_filter → uses existing MV path (backward compat).
	d := buildMultiMetricDashboard(t, nil)
	f := chartdata.ViewerFilters{PeriodPreset: "L12M"}
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)

	plan, err := chartdata.Plan(d, f, now)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(plan.SQL, "mv_bi_metric") {
		t.Error("single-metric plan should use MVs (backward compat)")
	}
}

func TestPlan_MultiMetric_EmptyMetrics_UsesMVs(t *testing.T) {
	// Empty include_metrics → treat as single-metric (MV path).
	d := buildMultiMetricDashboard(t, []string{})
	f := chartdata.ViewerFilters{PeriodPreset: "L12M"}
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)

	plan, err := chartdata.Plan(d, f, now)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(plan.SQL, "mv_bi_metric") {
		t.Error("empty metric_filter should fall back to MV path")
	}
}
