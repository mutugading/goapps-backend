-- Migration 000358: Switch EBITDA "YTD EBITDA" KPI from period="ytd" to "selected_ytd".
--
-- "ytd" always uses today's date as the anchor (Jan 1 2026 → today).
-- "selected_ytd" uses the viewer's selected month as the anchor:
--   May 2026 selected → Jan 1 2026 → May 31 2026  (compare via YTD_vs_LY → Jan-May 2025)
--   May 2025 selected → Jan 1 2025 → May 31 2025  (compare → Jan-May 2024)
--   no month selected → Jan 1 current_year → today (same as "ytd")
--
-- The compare mode stays "YTD_vs_LY" (shifts the YTD period back 1 year).

BEGIN;

-- kpi_config is stored in PostgreSQL as a raw JSON array [...] (not wrapped in {"items":[...]}).
-- The {"items":[...]} wrapper only exists in the gRPC proto/Struct layer.
-- bytesToMapList() in the repository decodes the JSONB column directly into []map[string]any.
-- Therefore: jsonb_agg() returns a plain array which is exactly the correct DB format.
-- Set selected_ytd for YTD EBITDA; remove explicit period from others → "selected" scope
-- (no period field = inherits viewer's period, fully dynamic with month picker).
UPDATE bi_dashboard
SET kpi_config = (
    SELECT jsonb_agg(
        CASE
            WHEN (kpi->>'label') = 'YTD EBITDA'
            THEN jsonb_set(kpi, '{period}', '"selected_ytd"')
            ELSE kpi - 'period'
        END
    )
    FROM jsonb_array_elements(kpi_config) AS kpi
)
WHERE dashboard_code = 'EBITDA'
  AND jsonb_typeof(kpi_config) = 'array';

COMMIT;
