-- 000400: Fix lm_code_field and lm_label_field — migration 000394 seeded camelCase
-- proto field names instead of actual snake_case DB column names, causing
-- ListMasterOptions to fail with "column not found" on all lookup dropdowns.
BEGIN;

UPDATE mst_lookup_master SET lm_code_field = 'mc_code',        lm_label_field = 'mc_name'        WHERE lm_code = 'MACHINE';
UPDATE mst_lookup_master SET lm_code_field = 'intm_code',      lm_label_field = 'intm_name'      WHERE lm_code = 'INTERMINGLING';
UPDATE mst_lookup_master SET lm_code_field = 'pg_code',        lm_label_field = 'pg_name'        WHERE lm_code = 'PRODUCT_GRADE';
UPDATE mst_lookup_master SET lm_code_field = 'mbh_mb_costing', lm_label_field = 'mbh_mgt_name'   WHERE lm_code = 'MB_HEAD';
UPDATE mst_lookup_master SET lm_code_field = 'bbc_code',       lm_label_field = 'bbc_name'       WHERE lm_code = 'BOX_BOBBIN_COST';
UPDATE mst_lookup_master SET lm_code_field = 'mbs_mb_costing', lm_label_field = 'mbs_mgt_name'   WHERE lm_code = 'MB_SPIN';

COMMIT;
