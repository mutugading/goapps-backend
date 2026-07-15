-- 000463 down: remove only oracle_csv composition rows + 202606 skip-log; leave the 9 admin test rows.
BEGIN;
DELETE FROM mst_mb_composition WHERE mbcm_created_by='oracle_csv';
DELETE FROM mst_mb_composition_import_skip WHERE mcis_period='202606';
COMMIT;
