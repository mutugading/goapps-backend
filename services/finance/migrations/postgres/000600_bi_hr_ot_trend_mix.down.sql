-- H036 (DOWN): buang kartu ot_mix dari HR_OT_TREND.
BEGIN;
UPDATE bi_dashboard
SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(layout_config->'secondary_charts') e
              WHERE e->>'chart_type' <> 'ot_mix'), '[]'::jsonb))
WHERE dashboard_code = 'HR_OT_TREND'
  AND layout_config->'secondary_charts' @> '[{"chart_type":"ot_mix"}]'::jsonb;
COMMIT;
