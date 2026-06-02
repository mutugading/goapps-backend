-- Migration 000352: Fix DELIVERY_MARGIN chart — add metric_filter so the main chart
-- queries bi_fact_metric directly for NETT_SALES and MARGIN only, instead of summing
-- ALL metric_names from mv_bi_metric_g1 (which doubles the value because NETT_SALES +
-- MARGIN + GROSS_SALES are separate rows with the same group_1/period).
--
-- With metric_filter set, the planner routes to planMultiMetric():
--   • One SQL SELECT per metric (NETT_SALES, MARGIN)
--   • UNION ALL joined by periode_date → two time-series lines
--   • Shape() produces 2 Series (Net Sales, Margin) with correct individual values
--
-- Also switch chart_type to LINE (4) so the frontend renders the two series as
-- two separate lines rather than a stacked-bar, which is not meaningful for
-- multi-metric data.
BEGIN;

UPDATE bi_dashboard
SET
  chart_type  = 4,   -- CHART_TYPE_LINE
  chart_config = chart_config
    || '{"metric_filter": {"include_metrics": ["NETT_SALES", "MARGIN"]}, "available_chart_types": ["line", "bar", "data_table"]}'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
