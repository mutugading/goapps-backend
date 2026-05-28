BEGIN;

UPDATE bi_job
   SET is_active = FALSE,
       config    = config - 'kind'
 WHERE job_name = 'MV_REFRESH';

COMMIT;
