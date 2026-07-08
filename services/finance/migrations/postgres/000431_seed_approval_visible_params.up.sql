-- Seed approval-review visibility (item #4) for the subset of the brief's
-- display-label list that could be confidently matched to existing
-- mst_parameter.param_code values.
--
-- IMPORTANT: param_code values were verified by dry-running the FULL
-- migration history (0 -> 431) on a throwaway database and querying the
-- resulting live mst_parameter table — NOT by reading a single seed
-- migration in isolation. A later Oracle-sync migration (000411
-- replace_seed_with_oracle_masters, which supersedes 000381's catalog)
-- renamed several codes: YARN_DENIER -> DENIER, FILAMENT_COUNT ->
-- NO_OF_FILAMENTS, WASTE_PCT -> WASTE_PERC. "LUSTRE_TYPE" (or any lustre/
-- luster param) does not exist anywhere in the final catalog at all.
-- Idempotent: UPDATE keyed on param_code, safe to re-run.
--
-- NOT matched (left is_approval_visible = FALSE, needs business input before
-- a future migration can wire them up):
--   - "item code", "name", "type"    -> likely core product identity fields
--                                        (cost_product.*), not mst_parameter rows.
--   - "lusture"                      -> no lustre/luster param exists anywhere
--                                        in the live catalog (verified via full
--                                        migration dry-run, not just grep).
--   - "product quality"              -> no single param maps cleanly; the
--                                        per-grade breakdown is already covered
--                                        by "quality Ax/ae/a9/a/b/c" below.
--   - "production"                   -> only appears as an owner_department
--                                        value ('Production'), not a param_code.
--   - "type of bobin"                -> candidates CAP_PACK_CODE / DEL_PACK_CODE
--                                        (Captive vs Delivery packing) — ambiguous
--                                        which context the brief means.
--   - "no of bobbin"                 -> candidates CAP_NO_OF_BOB / DEL_NO_OF_BOB —
--                                        same Captive/Delivery ambiguity.
--   - "box or jumbo or palet"        -> this concept lives on cost_product_spec
--                                        (cps_box_type IN ('JUMBO','NORMAL','PALLET')),
--                                        a different table entirely, not mst_parameter.

UPDATE mst_parameter p
   SET is_approval_visible    = TRUE,
       approval_display_order = v.approval_display_order
  FROM (VALUES
    ('MC_NAME',          10),  -- machine code
    ('MC_EFFICIENCY',    20),  -- machine efficiency
    ('MC_SPEED',         30),  -- machine speed
    ('TPM',              40),  -- machine tpm
    ('DENIER',           50),  -- product denier (was YARN_DENIER pre-Oracle-sync)
    ('ACT_DENIER',       60),  -- product actual denier
    ('NO_OF_PLY',        70),  -- product no of ply
    ('NO_OF_FILAMENTS',  80),  -- product no of filaments (was FILAMENT_COUNT pre-Oracle-sync)
    ('RM_TYPE',          90),  -- product rm type
    ('RAW_MATERIAL',    100),  -- product RM
    ('CROSS_SECTION',   110),  -- product cross section
    ('INTERMINGLE',     120),  -- product inter mingle
    ('WASTE_PERC',      130),  -- product waste percent (was WASTE_PCT pre-Oracle-sync)
    ('OPU',             140),  -- product opu
    ('AX_PERC',         150),  -- quality Ax
    ('AE_PERC',         160),  -- quality ae
    ('A9_PERC',         170),  -- quality a9
    ('A_PERC',          180),  -- quality a
    ('B_PERC',          190),  -- quality b
    ('C_PERC',          200),  -- quality c
    ('NET_BOB_WT',      210)   -- bobin weight
  ) AS v(param_code, approval_display_order)
 WHERE p.param_code = v.param_code
   AND p.deleted_at IS NULL;
