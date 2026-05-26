BEGIN;
DELETE FROM bi_fact_metric WHERE dimension_key = '__DEMO__';
REFRESH MATERIALIZED VIEW mv_bi_metric_g1;
REFRESH MATERIALIZED VIEW mv_bi_metric_g2;
COMMIT;
