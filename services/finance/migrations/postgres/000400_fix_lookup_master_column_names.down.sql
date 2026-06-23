-- Revert to original (wrong) camelCase values from 000394 seed.
BEGIN;

UPDATE mst_lookup_master SET lm_code_field = 'machineCode',       lm_label_field = 'machineName'       WHERE lm_code = 'MACHINE';
UPDATE mst_lookup_master SET lm_code_field = 'interminglingCode', lm_label_field = 'interminglingName' WHERE lm_code = 'INTERMINGLING';
UPDATE mst_lookup_master SET lm_code_field = 'pgCode',            lm_label_field = 'pgName'            WHERE lm_code = 'PRODUCT_GRADE';
UPDATE mst_lookup_master SET lm_code_field = 'mbhMbCosting',      lm_label_field = 'mbhMgtName'        WHERE lm_code = 'MB_HEAD';
UPDATE mst_lookup_master SET lm_code_field = 'bbcCode',           lm_label_field = 'bbcName'           WHERE lm_code = 'BOX_BOBBIN_COST';
UPDATE mst_lookup_master SET lm_code_field = '',                  lm_label_field = ''                  WHERE lm_code = 'MB_SPIN';

COMMIT;
