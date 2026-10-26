-- Kembalikan H042: hapus top_section dan dashboard sumber NIK (fakta HR_OT_OVER_NIK ikut dihapus).
BEGIN;
UPDATE bi_dashboard SET layout_config = layout_config - 'top_section' WHERE dashboard_code = 'HR_OT_OVERLIMIT';
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_OVER_NIK');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_OVER_NIK';
DELETE FROM bi_fact_metric WHERE type = 'HR_OT_OVER_NIK';
COMMIT;
