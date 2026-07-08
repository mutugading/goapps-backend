-- Replace cost_product_spec's paper-tube-type FK with a plain fixed enum
-- column (per product-request-workflow-revamp design.md §2 D3). Confirmed
-- via a full-repo sweep that cost_paper_tube_type has exactly one consumer
-- (this FK) and one UI flow — safe to sever the FK without dropping the
-- master table itself (that is a separate, deferred cleanup, out of scope
-- for this migration).
--
-- NOTE: design.md's sketch backfill referenced a column
-- "cptt_type_name" that does not exist on cost_paper_tube_type (actual
-- columns are cptt_code / cptt_display_name, see migration 000201) — this
-- migration matches against cptt_display_name instead, which carries the
-- same "3 inch jumbo" / "Pallet (no tube)" style human-readable text the
-- design intended to pattern-match on.

ALTER TABLE cost_product_spec ADD COLUMN IF NOT EXISTS cps_tube_type VARCHAR(10);
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_tube_type;
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_tube_type
    CHECK (cps_tube_type IS NULL OR cps_tube_type IN ('PAPER', 'PLASTIC'));

-- Backfill existing rows best-effort from the old FK's master data, if a
-- plausible name match exists (e.g. cptt_display_name ILIKE
-- '%paper%'/'%plastic%'); rows that don't match stay NULL (historical/
-- ambiguous — acceptable per D1's precedent of "unpopulated is fine for old
-- rows"). Today's seed data (000201) has no "plastic" tube type and none of
-- the display names mention "paper" either, so this backfill is expected to
-- leave all current rows NULL — it exists for forward/prod-data safety and
-- any future cost_paper_tube_type rows that do carry a matching name.
UPDATE cost_product_spec cps
    SET cps_tube_type = CASE
        WHEN cptt.cptt_display_name ILIKE '%plastic%' THEN 'PLASTIC'
        WHEN cptt.cptt_display_name ILIKE '%paper%' THEN 'PAPER'
        ELSE NULL
    END
    FROM cost_paper_tube_type cptt
    WHERE cps.cps_paper_tube_type_id = cptt.cptt_paper_tube_type_id;

ALTER TABLE cost_product_spec ALTER COLUMN cps_paper_tube_type_id DROP NOT NULL;
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS cost_product_spec_cps_paper_tube_type_id_fkey;

COMMENT ON COLUMN cost_product_spec.cps_paper_tube_type_id IS 'Deprecated for new writes (product-request-workflow-revamp D3) — retained for historical rows, no longer form-editable, FK dropped. Nullable.';
COMMENT ON COLUMN cost_product_spec.cps_tube_type IS 'Fixed Paper/Plastic enum replacing cps_paper_tube_type_id for new writes (product-request-workflow-revamp D3). NULL means not collected / ambiguous historical backfill.';
