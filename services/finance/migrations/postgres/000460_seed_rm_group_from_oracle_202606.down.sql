-- 000460 down: remove the 26 oracle_csv RM groups + their items (children first).
BEGIN;
DELETE FROM cst_rm_group_detail WHERE created_by='oracle_csv'
  AND group_head_id IN (SELECT group_head_id FROM cst_rm_group_head WHERE created_by='oracle_csv');
DELETE FROM cst_rm_group_head WHERE created_by='oracle_csv';
COMMIT;
