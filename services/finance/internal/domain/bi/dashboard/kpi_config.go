package dashboard

import (
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

// KpiEntry is one KPI card definition stored inside bi_dashboard.kpi_config.
type KpiEntry struct {
	Label            string
	ValueField       string
	Agg              string // sum|avg|min|max|last
	Compare          string // MoM|QoQ|YoY|YTD_vs_LY|none
	Format           chart.NumberFormat
	Decimals         int
	ShowSparkline    bool
	SparklinePeriods int
}

// KpiConfig is the ordered list of KPI cards a dashboard renders above its main chart.
type KpiConfig []KpiEntry

// allowedAggs is the closed set of aggregation keys.
var allowedAggs = map[string]struct{}{
	"sum": {}, "avg": {}, "min": {}, "max": {}, "last": {},
}

// allowedKpiCompares is the closed set of KPI compare keys (separate from chart compare set:
// KPI supports YTD_vs_LY which the chart overlay calls YTD).
var allowedKpiCompares = map[string]struct{}{
	"MoM": {}, "QoQ": {}, "YoY": {}, "YTD_vs_LY": {}, "none": {},
}

// ParseKpiConfig validates every entry in the raw list and returns a typed KpiConfig.
//
// An empty input yields an empty (but non-nil) KpiConfig with no error — a dashboard
// is permitted to render no KPI cards above its main chart.
func ParseKpiConfig(raw []map[string]any) (KpiConfig, error) {
	if len(raw) == 0 {
		return KpiConfig{}, nil
	}
	if len(raw) > 6 {
		return nil, fmt.Errorf("%w: maximum 6 KPI cards, got %d", ErrInvalidKpiConfig, len(raw))
	}
	out := make(KpiConfig, 0, len(raw))
	for i, m := range raw {
		entry, err := parseKpiEntry(m, i)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// parseKpiEntry validates one KPI entry map.
func parseKpiEntry(m map[string]any, idx int) (KpiEntry, error) {
	label, _ := m["label"].(string)
	if label == "" {
		return KpiEntry{}, fmt.Errorf("%w: entry %d missing 'label'", ErrInvalidKpiConfig, idx)
	}

	valueField, _ := m["value_field"].(string)
	if valueField == "" {
		return KpiEntry{}, fmt.Errorf("%w: entry %d missing 'value_field'", ErrInvalidKpiConfig, idx)
	}

	agg, _ := m["agg"].(string)
	if agg == "" {
		agg = "sum"
	}
	if _, ok := allowedAggs[agg]; !ok {
		return KpiEntry{}, fmt.Errorf("%w: entry %d agg %q is not one of sum/avg/min/max/last", ErrInvalidKpiConfig, idx, agg)
	}

	compare, _ := m["compare"].(string)
	if compare == "" {
		compare = "none"
	}
	if _, ok := allowedKpiCompares[compare]; !ok {
		return KpiEntry{}, fmt.Errorf("%w: entry %d compare %q is not one of MoM/QoQ/YoY/YTD_vs_LY/none", ErrInvalidKpiConfig, idx, compare)
	}

	format, _ := m["format"].(string)
	if format == "" {
		format = string(chart.NumberFormatCurrencyThousands)
	}
	if !chart.IsValidNumberFormat(format) {
		return KpiEntry{}, fmt.Errorf("%w: entry %d format %q is not a recognised number-format", ErrInvalidKpiConfig, idx, format)
	}

	decimals := 0
	if v, ok := mergedAsInt(m, "decimals"); ok {
		decimals = v
	}
	if decimals < 0 || decimals > 6 {
		return KpiEntry{}, fmt.Errorf("%w: entry %d decimals %d out of [0,6]", ErrInvalidKpiConfig, idx, decimals)
	}

	showSparkline, _ := m["show_sparkline"].(bool)
	sparkPeriods := 0
	if v, ok := mergedAsInt(m, "sparkline_periods"); ok {
		sparkPeriods = v
	}
	if showSparkline && sparkPeriods == 0 {
		sparkPeriods = 12
	}

	return KpiEntry{
		Label:            label,
		ValueField:       valueField,
		Agg:              agg,
		Compare:          compare,
		Format:           chart.NumberFormat(format),
		Decimals:         decimals,
		ShowSparkline:    showSparkline,
		SparklinePeriods: sparkPeriods,
	}, nil
}

// MarshalToList converts a KpiConfig back to a JSON-friendly list of maps.
func (k KpiConfig) MarshalToList() []map[string]any {
	out := make([]map[string]any, len(k))
	for i, e := range k {
		m := map[string]any{
			"label":       e.Label,
			"value_field": e.ValueField,
			"agg":         e.Agg,
			"compare":     e.Compare,
			"format":      string(e.Format),
		}
		if e.Decimals != 0 {
			m["decimals"] = e.Decimals
		}
		if e.ShowSparkline {
			m["show_sparkline"] = true
			if e.SparklinePeriods != 0 {
				m["sparkline_periods"] = e.SparklinePeriods
			}
		}
		out[i] = m
	}
	return out
}
