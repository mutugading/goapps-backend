-- H037 (DOWN): buang kartu ot_pareto dari HR_OT_TREND dan dashboard pendamping HR_OT_PARETO.
BEGIN;
UPDATE bi_dashboard
SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(layout_config->'secondary_charts') e
              WHERE e->>'chart_type' <> 'ot_pareto'), '[]'::jsonb))
WHERE dashboard_code = 'HR_OT_TREND'
  AND layout_config->'secondary_charts' @> '[{"chart_type":"ot_pareto"}]'::jsonb;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_PARETO');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_PARETO';
COMMIT;
