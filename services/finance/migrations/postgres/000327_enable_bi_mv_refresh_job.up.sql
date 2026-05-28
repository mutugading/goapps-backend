-- Re-enable the MV_REFRESH job and tag it with kind=mv_refresh so the
-- TriggerHandler can skip the Oracle-fetch path and only refresh the MVs.
BEGIN;

UPDATE bi_job
   SET is_active = TRUE,
       config    = COALESCE(config, '{}'::jsonb) || '{"kind":"mv_refresh"}'::jsonb
 WHERE job_name = 'MV_REFRESH';

COMMIT;
