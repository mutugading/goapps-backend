-- Seed: 24 months of demo fact data for EBITDA + NET PROFIT (tagged dimension_key='__DEMO__' for easy removal).
BEGIN;

WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'EXCEL_UPLOAD'),
months AS (
    SELECT generate_series(
        date_trunc('month', NOW() - interval '23 months')::date,
        date_trunc('month', NOW())::date,
        interval '1 month'
    )::date AS d
),
buckets AS (
    SELECT * FROM (VALUES
        ('EBITDA',     'INCOME',               10,  5600000.00, 0.05),
        ('EBITDA',     'PRODUCTION COST',      20, -2700000.00, 0.08),
        ('EBITDA',     'COLOR CONSUMPTION',    30,  -180000.00, 0.10),
        ('EBITDA',     'MATERIAL CONSUMPTION', 40,  -480000.00, 0.10),
        ('EBITDA',     'ENERGY COST',          50,  -540000.00, 0.07),
        ('EBITDA',     'MANPOWER',             60,  -950000.00, 0.04),
        ('EBITDA',     'OVERHEADS',            70,  -110000.00, 0.05),
        ('EBITDA',     'SELLING COST',         80,  -175000.00, 0.06),
        ('NET PROFIT', 'OPERATING PROFIT',     10,   400000.00, 0.10),
        ('NET PROFIT', 'FINANCE COST',         20,   -40000.00, 0.15),
        ('NET PROFIT', 'TAX',                  30,   -90000.00, 0.05),
        ('NET PROFIT', 'OTHER',                40,    20000.00, 0.20)
    ) AS t(group_1, group_2, ord, base_val, jitter)
),
computed AS (
    SELECT
        m.d,
        b.group_1,
        b.group_2,
        b.ord,
        ROUND((b.base_val * (1 + (random() - 0.5) * b.jitter))::numeric, 2) AS raw_value
    FROM months m CROSS JOIN buckets b
)
INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3,
    group_1_order, group_2_order, group_3_order,
    periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, source_id, dimension_key, loaded_at
)
SELECT
    'MIS',
    c.group_1,
    c.group_2,
    NULL,
    CASE c.group_1 WHEN 'EBITDA' THEN 10 ELSE 20 END,
    c.ord,
    NULL,
    'MONTHLY',
    c.d,
    TO_CHAR(c.d, 'YYYYMM'),
    c.raw_value,
    bi_compute_display_value('MIS', c.group_1, c.group_2, c.raw_value),
    'USD',
    'ACTUAL',
    (SELECT source_id FROM src),
    '__DEMO__',
    NOW()
FROM computed c
ON CONFLICT (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key) DO NOTHING;

REFRESH MATERIALIZED VIEW mv_bi_metric_g1;
REFRESH MATERIALIZED VIEW mv_bi_metric_g2;

COMMIT;
