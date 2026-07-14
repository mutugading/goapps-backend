-- Widen cal_job.cj_scope to add MB_BATCH, so the MB_BATCH cost calc trigger (mbbatch.Service)
-- can track its runs through the existing cal_job/JobRepository machinery instead of a new table.
ALTER TABLE cal_job DROP CONSTRAINT IF EXISTS chk_cj_scope;
ALTER TABLE cal_job ADD CONSTRAINT chk_cj_scope
    CHECK (cj_scope IN ('ALL','FILTERED','SINGLE_PRODUCT','SINGLE_ROUTE','MB_BATCH'));
