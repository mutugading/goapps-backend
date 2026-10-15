package chartdata_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

func hrDashboard(t *testing.T, filterType, group1 string) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "HR_TEST", Title: "HR", FilterType: filterType, FilterGroup1: group1,
		PeriodGrain: "MONTHLY", DefaultPeriod: "L12M", ChartType: "bar",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"},
		CompareModes:   []string{"YoY"}, MaxDrillLevel: 3, CacheTTLSec: 60,
		GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// HD-07: group_3 (NIK) of HR_* types is refused server-side without the employee permission.
func TestGetDataHandler_EmployeeLevel(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name       string
		filterType string
		group1     string
		drill      []string
		can, super bool
		wantDenied bool
	}{
		{"HR depth0 tanpa izin ok", "HR_OT_HOURS", "", nil, false, false, false},
		{"HR depth1 (group_2) tanpa izin ok", "HR_OT_HOURS", "", []string{"PRD"}, false, false, false},
		{"HR depth2 (group_3) tanpa izin ditolak", "HR_OT_HOURS", "", []string{"PRD", "LEMBUR"}, false, false, true},
		{"HR depth2 dengan izin ok", "HR_OT_HOURS", "", []string{"PRD", "LEMBUR"}, true, false, false},
		{"HR depth2 super admin ok", "HR_OT_HOURS", "", []string{"PRD", "LEMBUR"}, false, true, false},
		{"HR group_1 terpasang, depth1 = group_3, ditolak", "HR_OT_HOURS", "PRD", []string{"LEMBUR"}, false, false, true},
		{"HR group_1 terpasang, depth0 ok", "HR_OT_HOURS", "PRD", nil, false, false, false},
		{"type non-HR tidak dibatasi", "MIS", "", []string{"A", "B"}, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dash := hrDashboard(t, c.filterType, c.group1)
			repo := &mockDashboardRepo{getByCode: func(context.Context, string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
			fact := &mockFactRepo{}
			h := chartdata.NewGetDataHandler(repo, fact, newMockCache(), func(any) string { return "h" }).WithNow(now)
			_, err := h.Handle(context.Background(), chartdata.GetDataQuery{
				DashboardCode: "HR_TEST",
				Filters:       chartdata.ViewerFilters{PeriodPreset: "L12M", DrillPath: c.drill},
				IsSuperAdmin:  c.super, CanViewEmployee: c.can,
			})
			denied := errors.Is(err, dashboarddomain.ErrForbidden)
			if denied != c.wantDenied {
				t.Fatalf("denied=%v want %v (err=%v)", denied, c.wantDenied, err)
			}
			if c.wantDenied && fact.queries != 0 {
				t.Fatalf("query ke DB tidak boleh jalan saat ditolak, queries=%d", fact.queries)
			}
		})
	}
}

func TestRequestsEmployeeLevel_ComputedRatioGroup3(t *testing.T) {
	d := hrDashboard(t, "HR_SICK_DAYS", "")
	f := chartdata.ViewerFilters{ComputedRatio: &dashboarddomain.ComputedRatioConfig{Numerator: "A", GroupBy: "group_3"}}
	if !chartdata.RequestsEmployeeLevel(d, f) {
		t.Fatal("computed ratio group_3 pada HR_* harus dianggap level karyawan")
	}
	f.ComputedRatio.GroupBy = "group_2"
	if chartdata.RequestsEmployeeLevel(d, f) {
		t.Fatal("group_2 bukan level karyawan")
	}
}

func TestRequestsEmployeeLevel_GroupByVariantsAndHRCase(t *testing.T) {
	for _, g := range []string{"GROUP_3", " group_3 ", "Group_3"} {
		d := hrDashboard(t, "HR_SICK_DAYS", "")
		f := chartdata.ViewerFilters{ComputedRatio: &dashboarddomain.ComputedRatioConfig{Numerator: "A", GroupBy: g}}
		if !chartdata.RequestsEmployeeLevel(d, f) {
			t.Fatalf("group_by %q harus dianggap level karyawan", g)
		}
	}
	if !chartdata.IsHRType(" hr_ot_hours") || chartdata.IsHRType("MIS") {
		t.Fatal("IsHRType case-insensitive & trim")
	}
}

func TestNormalizeGroupBy(t *testing.T) {
	for in, want := range map[string]string{"": "group_2", "GROUP_1": "group_1", " group_3 ": "group_3"} {
		got, err := chartdata.NormalizeGroupBy(in)
		if err != nil || got != want {
			t.Fatalf("%q → %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"group_3; DROP TABLE x", "type", "group_4", "1=1"} {
		if _, err := chartdata.NormalizeGroupBy(bad); !errors.Is(err, factmetric.ErrInvalidPlan) {
			t.Fatalf("%q harus ditolak, err=%v", bad, err)
		}
	}
}
