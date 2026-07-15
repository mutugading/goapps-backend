-- 000462 down: delete only the oracle_csv-seeded heads (leave the pre-existing ~4180).
-- Defensive: skip (do not delete) any seeded head still referenced by mst_mb_spin or
-- mst_mb_composition, to avoid an FK failure. On real dev these heads have no dependents
-- (they didn't exist when mst_mb_spin was seeded), so this deletes all 23; the guard only
-- matters on a from-zero disposable DB where 000418 may have linked a spin to a matching
-- oracle_sys_id. Roll back 000463 (composition) first for a full teardown.
BEGIN;
DELETE FROM mst_mb_head h WHERE h.created_by='oracle_csv'
  AND NOT EXISTS (SELECT 1 FROM mst_mb_spin s WHERE s.mbs_mbh_id = h.mbh_id)
  AND NOT EXISTS (SELECT 1 FROM mst_mb_composition c WHERE c.mbcm_mbh_id = h.mbh_id)
  AND NOT EXISTS (SELECT 1 FROM mst_mb_composition c WHERE c.mbcm_mb_ref_mbh_id = h.mbh_id);
DO $$
DECLARE remaining INTEGER;
BEGIN
  SELECT count(*) INTO remaining FROM mst_mb_head WHERE created_by='oracle_csv';
  IF remaining > 0 THEN
    RAISE NOTICE '000462 down: % oracle_csv head(s) kept (still referenced by spin/composition) — remove dependents first for a full teardown', remaining;
  END IF;
END $$;
COMMIT;
