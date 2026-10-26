-- H039 (DOWN): buang dashboard kualitas data OT dan fakta NIK terdampak.
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP'));
DELETE FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP');
DELETE FROM bi_fact_metric WHERE type = 'HR_OT_DQ_EMP';
COMMIT;
