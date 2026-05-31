-- Schema v1.1: multi-metric support (additive, backward-compatible).
-- Existing EBITDA rows get defaults: metric_name='VALUE', metric_category='VALUE', agg_method='SUM'.
-- NULLS NOT DISTINCT is preserved; metric_name added to business key.
BEGIN;

ALTER TABLE bi_fact_metric
  ADD COLUMN IF NOT EXISTS metric_name     VARCHAR(50) NOT NULL DEFAULT 'VALUE',
  ADD COLUMN IF NOT EXISTS metric_category VARCHAR(20) NOT NULL DEFAULT 'VALUE',
  ADD COLUMN IF NOT EXISTS agg_method      VARCHAR(20) NOT NULL DEFAULT 'SUM';

COMMENT ON COLUMN bi_fact_metric.metric_name IS
  'Specific KPI within a dimension combo (UPPERCASE_SNAKE_CASE). Default ''VALUE'' for EBITDA/P&L pattern. Multi-metric modules (SALES) use GROSS_SALES, MARGIN, etc.';
COMMENT ON COLUMN bi_fact_metric.metric_category IS
  'Classification: VOLUME (PCS/HOURS), VALUE (currency), AVERAGE (per-unit), RATIO (%), DERIVED (computed).';
COMMENT ON COLUMN bi_fact_metric.agg_method IS
  'How to roll up: SUM (default), WEIGHTED_AVG, AVG, LAST, RATIO. MVs only include SUM rows.';

-- Drop old constraint (does not include metric_name).
ALTER TABLE bi_fact_metric
  DROP CONSTRAINT IF EXISTS uq_bi_fm_business_key;

-- Recreate with metric_name — allows 6 metric rows per dimension combo (delivery margin pattern).
ALTER TABLE bi_fact_metric
  ADD CONSTRAINT uq_bi_fm_business_key UNIQUE NULLS NOT DISTINCT
  (type, group_1, group_2, group_3, periode_grain, periode_date, metric_name, scenario, dimension_key);

CREATE INDEX IF NOT EXISTS idx_bi_fm_metric_name
  ON bi_fact_metric (type, metric_name, periode_date) WHERE is_active;

COMMIT;
