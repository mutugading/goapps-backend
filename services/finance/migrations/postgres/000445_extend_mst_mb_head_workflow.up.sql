-- MB Costing Suite: new workflow-state + recipe-identity columns on mst_mb_head.
-- mbh_entry_status is DISTINCT from legacy mbh_status/mbh_check_status (frozen Oracle passthrough) —
-- never read/written by the same code path. mbh_vs_number (added 000416) and mbh_status are
-- pre-existing and are NOT re-added here.
ALTER TABLE mst_mb_head
  ADD COLUMN IF NOT EXISTS mbh_entry_status VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
    CHECK (mbh_entry_status IN ('DRAFT','SUBMITTED','APPROVED','VALIDATED','UN_APPROVED','REVOKED')),
  ADD COLUMN IF NOT EXISTS mbh_is_boughtout BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS mbh_current_version INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS mbh_machine_fixed_total NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_state_reason TEXT,
  ADD COLUMN IF NOT EXISTS mbh_dev_code VARCHAR(50),
  ADD COLUMN IF NOT EXISTS mbh_shade_code VARCHAR(20),
  ADD COLUMN IF NOT EXISTS mbh_shade_name VARCHAR(100),
  ADD COLUMN IF NOT EXISTS mbh_cross_section VARCHAR(20),
  ADD COLUMN IF NOT EXISTS mbh_lusture_code VARCHAR(10),
  ADD COLUMN IF NOT EXISTS mbh_cost_product_id BIGINT,
  ADD COLUMN IF NOT EXISTS mbh_cost_generated_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS mbh_cost_generated_by VARCHAR(100),
  ADD COLUMN IF NOT EXISTS mbh_param_waste NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_quality_loss NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_efficiency NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_dev_expense NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_packing NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_mb_prod_per_day NUMERIC(20,6),
  ADD COLUMN IF NOT EXISTS mbh_param_throughput_per_hour VARCHAR(10),
  ADD COLUMN IF NOT EXISTS mbh_param_no_of_process VARCHAR(10);

CREATE INDEX IF NOT EXISTS idx_mbh_entry_status ON mst_mb_head (mbh_entry_status);
CREATE INDEX IF NOT EXISTS idx_mbh_lusture_code ON mst_mb_head (mbh_lusture_code) WHERE mbh_lusture_code IS NOT NULL;

COMMENT ON COLUMN mst_mb_head.mbh_entry_status IS 'New MB Costing workflow state — distinct from legacy mbh_status/mbh_check_status, never confused with those';
COMMENT ON COLUMN mst_mb_head.mbh_lusture_code IS 'Soft-link to mst_mb_lusture.mbl_code — distinct from legacy free-text mbh_lesture column';
