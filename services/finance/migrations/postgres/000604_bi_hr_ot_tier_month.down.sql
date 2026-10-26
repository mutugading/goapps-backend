-- H040 (DOWN): buang dashboard tier OT dan fakta HR_OT_TIER.
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TIER_MONTH');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TIER_MONTH';
DELETE FROM bi_fact_metric WHERE type = 'HR_OT_TIER';
COMMIT;
