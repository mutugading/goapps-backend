-- Split cost_product_spec's single free-text shade field into a code/name
-- pair, mirroring cost_product_master's established cpm_shade_code +
-- cpm_shade_name split (per product-request-workflow-revamp design.md §2
-- D2). The master-shade lookup path (cps_shade_id) was always dead code —
-- the form only ever sent shadeId: 0 and populated the free-text column —
-- so cps_shade_custom_text is renamed to cps_shade_code (same semantics,
-- clearer name) and a new nullable cps_shade_name column is added.

ALTER TABLE cost_product_spec RENAME COLUMN cps_shade_custom_text TO cps_shade_code;
ALTER TABLE cost_product_spec ADD COLUMN cps_shade_name VARCHAR(100);

ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_shade_present;
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_shade_present CHECK (
    cps_shade_id IS NOT NULL OR cps_shade_code IS NOT NULL
);

COMMENT ON COLUMN cost_product_spec.cps_shade_code IS 'Free-text or master shade code (e.g. NL, Z114S) — renamed from cps_shade_custom_text.';
COMMENT ON COLUMN cost_product_spec.cps_shade_name IS 'Human-readable shade name (e.g. Natural, Jet Black), mirrors cost_product_master.cpm_shade_name.';
