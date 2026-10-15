-- HD-05: grain WEEKLY (periode_date = Senin, label IYYY-"W"IW).
ALTER TABLE bi_dashboard DROP CONSTRAINT IF EXISTS bi_dashboard_periode_grain_check;
ALTER TABLE bi_dashboard ADD CONSTRAINT bi_dashboard_periode_grain_check
    CHECK (periode_grain IN ('DAILY','WEEKLY','MONTHLY','QUARTERLY','YEARLY'));

ALTER TABLE bi_fact_metric DROP CONSTRAINT IF EXISTS bi_fact_metric_periode_grain_check;
ALTER TABLE bi_fact_metric ADD CONSTRAINT bi_fact_metric_periode_grain_check
    CHECK (periode_grain IN ('DAILY','WEEKLY','MONTHLY','QUARTERLY','YEARLY'));
