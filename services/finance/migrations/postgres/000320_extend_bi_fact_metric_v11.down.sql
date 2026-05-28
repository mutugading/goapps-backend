BEGIN;

ALTER TABLE bi_fact_metric DROP CONSTRAINT IF EXISTS uq_bi_fm_business_key;

ALTER TABLE bi_fact_metric
  DROP COLUMN IF EXISTS metric_name,
  DROP COLUMN IF EXISTS metric_category,
  DROP COLUMN IF EXISTS agg_method;

DROP INDEX IF EXISTS idx_bi_fm_metric_name;

-- Restore original constraint (without metric_name).
ALTER TABLE bi_fact_metric
  ADD CONSTRAINT uq_bi_fm_business_key UNIQUE NULLS NOT DISTINCT
  (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key);

COMMIT;
