-- HD-21 lanjutan: selisih |calc − tier 1+2+3| tidak berarti lagi setelah tier dicabut (000605), dan data lama
-- selalu selisih 0. Cabut dashboard HR_OT_DQ_MONTH (tab "Kualitas Data", 000603) dan HR_OT_DQ_EMP (sumber kartunya).
-- Fakta HR_OT_DQ / HR_OT_DQ_EMP tetap dihitung ETL (dipakai rekon R1). Idempotent.
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP'));
DELETE FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP');
COMMIT;
