-- 000608 - Audit table for Product Master ETL runs (Oracle items -> cost_product_master).
-- A PREVIEW run stores per-row classification; an APPLY run references its preview
-- (cper_preview_run_id doubles as the run token). Schema only.

BEGIN;

CREATE TABLE IF NOT EXISTS cost_product_etl_run (
    cper_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cper_run_type       VARCHAR(10)  NOT NULL CHECK (cper_run_type IN ('PREVIEW', 'APPLY')),
    cper_preview_run_id UUID NULL REFERENCES cost_product_etl_run (cper_id),
    cper_from_date      DATE         NOT NULL,
    cper_search         VARCHAR(200) NULL,
    cper_params         JSONB        NOT NULL DEFAULT '{}',
    cper_counts         JSONB        NOT NULL DEFAULT '{}',
    cper_rows           JSONB        NOT NULL DEFAULT '[]',
    cper_status         VARCHAR(20)  NOT NULL DEFAULT 'COMPLETED' CHECK (cper_status IN ('COMPLETED', 'FAILED', 'PARTIAL')),
    cper_error          TEXT         NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by          VARCHAR(100) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cper_created_at ON cost_product_etl_run (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cper_preview_run ON cost_product_etl_run (cper_preview_run_id) WHERE cper_preview_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cper_type_from_date ON cost_product_etl_run (cper_run_type, cper_from_date);

COMMENT ON TABLE cost_product_etl_run IS 'Audit of Product Master ETL preview/apply runs (Oracle items -> cost_product_master); counts and per-row results stored as JSONB.';

COMMIT;
