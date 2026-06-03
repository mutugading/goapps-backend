-- Revert 000358: restore period="ytd" on YTD EBITDA KPI.
BEGIN;

-- Restore period="ytd" for YTD EBITDA; leave others without explicit period (selected scope).
UPDATE bi_dashboard
SET kpi_config = (
    SELECT jsonb_agg(
        CASE
            WHEN (kpi->>'label') = 'YTD EBITDA'
            THEN jsonb_set(kpi, '{period}', '"ytd"')
            ELSE kpi - 'period'
        END
    )
    FROM jsonb_array_elements(kpi_config) AS kpi
)
WHERE dashboard_code = 'EBITDA'
  AND jsonb_typeof(kpi_config) = 'array';

COMMIT;
