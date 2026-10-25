-- H041: tier OT 1/2/3 tidak dibutuhkan manajemen (HD-21). Cabut dashboard HR_OT_TIER_MONTH (000569) beserta
-- fakta HR_OT_TIER. Kolom tier di staging tetap (dipakai rekon R1). Izin finance.bi.workforce.view dipakai
-- dashboard WORKFORCE lain, jadi tidak dicabut.
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TIER_MONTH');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_TIER_MONTH';
DELETE FROM bi_fact_metric WHERE type = 'HR_OT_TIER';
COMMIT;
