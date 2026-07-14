-- MB Costing Suite: periodic active-cost cache, populated ONLY via Push-to-Head execute.
-- The only table downstream consumers (POY, etc.) ever read from for MB cost.
CREATE TABLE IF NOT EXISTS cst_mb_cost (
  mbc_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbc_mbh_id            UUID NOT NULL,
  mbc_period            VARCHAR(6) NOT NULL CHECK (mbc_period ~ '^[0-9]{6}$'),
  mbc_cost_type         VARCHAR(20) NOT NULL CHECK (mbc_cost_type IN ('ACTUAL','SELLING','FORECAST')),
  mbc_cost_value        NUMERIC(20,6) NOT NULL,
  mbc_source_cpc_id     BIGINT,
  mbc_pushed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbc_pushed_by         VARCHAR(20) NOT NULL,
  mbc_is_active         BOOLEAN NOT NULL DEFAULT TRUE,
  mbc_created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbc_updated_at        TIMESTAMPTZ,
  mbc_updated_by        VARCHAR(20),
  CONSTRAINT fk_mbc_mbh FOREIGN KEY (mbc_mbh_id) REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE,
  CONSTRAINT fk_mbc_cpc FOREIGN KEY (mbc_source_cpc_id) REFERENCES cst_product_cost (cpc_cost_id) ON DELETE SET NULL,
  CONSTRAINT uq_mbc_period_type UNIQUE (mbc_mbh_id, mbc_period, mbc_cost_type)
);

CREATE INDEX IF NOT EXISTS idx_mbc_lookup ON cst_mb_cost (mbc_mbh_id, mbc_cost_type, mbc_period DESC) WHERE mbc_is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_mbc_period ON cst_mb_cost (mbc_period);
CREATE INDEX IF NOT EXISTS idx_mbc_pushed_at ON cst_mb_cost (mbc_pushed_at DESC);

COMMENT ON TABLE cst_mb_cost IS 'Periodic active-cost cache — the ONLY table downstream consumers (POY etc.) read for MB cost; populated exclusively by Push-to-Head execute';
