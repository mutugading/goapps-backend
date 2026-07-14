DROP INDEX IF EXISTS uq_mbcm_seq_active;

ALTER TABLE mst_mb_composition
  ADD CONSTRAINT uq_mbcm_seq UNIQUE (mbcm_mbh_id, mbcm_seq_no);
