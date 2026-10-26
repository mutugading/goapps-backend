-- H038 (DOWN): buang dashboard over-limit bulanan dan faktanya.
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP'));
DELETE FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP');
DELETE FROM bi_fact_metric WHERE type IN ('HR_OT_OVER_WEEKS', 'HR_OT_OVER_EMP') AND periode_grain = 'MONTHLY';
COMMIT;
