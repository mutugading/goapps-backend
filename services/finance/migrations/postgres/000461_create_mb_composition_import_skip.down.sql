-- 000461 down: drop the composition import skip-log table.
BEGIN;

DROP TABLE IF EXISTS mst_mb_composition_import_skip;

COMMIT;
