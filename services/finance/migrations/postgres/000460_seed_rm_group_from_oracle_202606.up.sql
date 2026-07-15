-- 000460: Seed 26 missing RM groups + items from Oracle MGTAPPS.CST_GRP_HEAD/CST_GRP_ITEM (period 202606 dependency).
-- group_code = CGH_SYS_ID. Cost fields default 0 (calc engine computes the 3 values later).
-- Additive + idempotent (ON CONFLICT DO NOTHING). Mirrors 000417 staging pattern.
BEGIN;

CREATE TEMP TABLE _stg_rmg_head (
  group_code VARCHAR(30), group_name VARCHAR(200), description TEXT,
  colourant VARCHAR(30), ci_name VARCHAR(30), is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_head (group_code, group_name, description, colourant, ci_name, is_active) VALUES
('202211651', 'RED MGTP-3149C', 'RED MGTP-3149C [legacy_code=PIG0000041]', 'Red MGTP-3149C', 'Pigment Red 149', TRUE),
('202303675', 'BLANC FIXE HXM', 'BLANC FIXE HXM [legacy_code=BLANC FIXE HXM]', NULL, NULL, TRUE),
('202411810', 'HELIOGEN BLUE K6912FP (BASF)', 'HELIOGEN BLUE K6912FP (BASF) [legacy_code=HELIOGEN BLUE K6912FP (BASF)]', NULL, NULL, TRUE),
('202411815', 'MASTERSET PES 1272', 'MASTERSET PES 1272 [legacy_code=MASTERSET PES 1272]', NULL, NULL, TRUE),
('202509875', 'SICOTRANS ROT K 2915', 'SICOTRANS ROT K 2915 [legacy_code=SICOTRANS ROT K 2915]', 'SICOTRANS ROT K 2915', NULL, TRUE),
('202411811', 'PALIOTOL YELLOW K 18100 UL (BASF)', 'PALIOTOL YELLOW K 18100 UL (BASF) [legacy_code=PALIOTOL YELLOW K18100]', 'PALIOTOL YELLOW K 18100 UL (BA', NULL, TRUE),
('202411812', 'HELIOGEN GREEN K 8730 FP ( BASF)', 'HELIOGEN GREEN K 8730 FP ( BASF) [legacy_code=HELIOGEN GREEN K 8730]', 'Heliogen Green K 8730 FP ( BAS', NULL, TRUE),
('202511901', 'MASTERSET PES 1336 UV-SETAS', 'MASTERSET PES 1336 UV-SETAS [legacy_code=UV MB SETAS]', NULL, NULL, TRUE),
('202512909', 'MASTERSET PES 1545 (SETAS-BSD-FR)', 'MASTERSET PES 1545 (SETAS-BSD-FR) [legacy_code=BLACK FR 1545]', NULL, NULL, TRUE),
('202606935', 'CROMAPTHAL VIOLET 5800', 'CROMAPTHAL VIOLET 5800 [legacy_code=CROMAPTHAL VIOLET 5800]', NULL, NULL, TRUE),
('202606936', 'PES 1561 BLACK+ ANTIMICROBIAL', 'PES 1561 BLACK+ ANTIMICROBIAL [legacy_code=PES 1561 BLACK+ ANTIMICROBIAL]', NULL, NULL, TRUE),
('202606937', 'PES 1273  WHITE ANTIMICROBIAL', 'PES 1273  WHITE ANTIMICROBIAL [legacy_code=PES 1273  WHITE ANTIMICROBIAL]', NULL, NULL, TRUE),
('202302665', 'MGT LIGHT COLOUR', 'MGT LIGHT COLOUR [legacy_code=LIGHT]', NULL, NULL, TRUE),
('202302666', 'MGT MEDIUM COLOUR', 'MGT MEDIUM COLOUR [legacy_code=MEDIUM]', NULL, NULL, TRUE),
('202302667', 'MGT DARK COLOUR', 'MGT DARK COLOUR [legacy_code=DARK]', NULL, NULL, TRUE),
('202302668', 'MGT EXTRA DARK COLOUR', 'MGT EXTRA DARK COLOUR [legacy_code=EXTRA DARK]', NULL, NULL, TRUE),
('202403761', 'PE GIALLO 2028', 'PE GIALLO 2028 [legacy_code=MBC0000116]', 'PE Giallo 2028', NULL, TRUE),
('202408790', 'HOSTASTAT FE 20 LIQUID', 'HOSTASTAT FE 20 LIQUID [legacy_code=HOSTASTAT]', NULL, NULL, TRUE),
('202411807', 'ORACET YELLOW 140 NQ', 'ORACET YELLOW 140 NQ [legacy_code=ORACET YELLOW 140 NQ]', 'Oracet Yellow 140 NQ', NULL, TRUE),
('202411808', 'PALIOGEN RED K3911', 'PALIOGEN RED K3911 [legacy_code=PALIOGEN RED K3911]', 'Paliogen Red K3911', NULL, TRUE),
('202412821', 'CINGUASIA PINK K4430 FP (BASF)', 'CINGUASIA PINK K4430 FP (BASF) [legacy_code=CINGUASIA PINK K4430 FP (BASF)]', 'Cinguasia Pink K4430 FP (BASF)', NULL, TRUE),
('202506861', 'UV-PES-1879-GREEN 66BT', 'UV-PES-1879-GREEN 66BT [legacy_code=UV-PES-1879]', NULL, NULL, TRUE),
('202506866', 'TINUVIN 1600 BASF', 'TINUVIN 1600 BASF [legacy_code=TINUVIN 1600 BASF]', NULL, NULL, TRUE),
('202209635', 'IRGASTAB 8201P', 'IRGASTAB 8201P [legacy_code=IRGASTAB 8201P]', 'Irgastab 8201P', NULL, TRUE),
('202409794', 'UV MB (TURKEY)', 'UV MB (TURKEY) [legacy_code=UV MB (TURKEY)]', NULL, NULL, TRUE),
('202504845', 'ORACET YELLOW 144 FE', 'ORACET YELLOW 144 FE [legacy_code=ORACET YELLOW 144 FE]', 'ORACET YELLOW 144 FE', NULL, TRUE);

INSERT INTO cst_rm_group_head
  (group_code, group_name, description, colourant, ci_name, is_active, created_by)
SELECT s.group_code, s.group_name, s.description, s.colourant, s.ci_name, s.is_active, 'oracle_csv'
FROM _stg_rmg_head s
ON CONFLICT (group_code) WHERE deleted_at IS NULL DO NOTHING;

CREATE TEMP TABLE _stg_rmg_item (
  group_code VARCHAR(30), item_code VARCHAR(20), grade_code VARCHAR(40),
  is_dummy BOOLEAN, is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_item (group_code, item_code, grade_code, is_dummy, is_active) VALUES
('202209635', '', 'NA', TRUE, FALSE),
('202211651', 'PIG0000041', 'BSF', FALSE, TRUE),
('202302665', '', 'NA', TRUE, FALSE),
('202302666', '', 'NA', TRUE, FALSE),
('202302667', '', 'NA', TRUE, FALSE),
('202302668', '', 'NA', TRUE, FALSE),
('202303675', '', 'NA', TRUE, FALSE),
('202409794', '', 'NA', TRUE, FALSE),
('202411810', '', 'NA', TRUE, FALSE),
('202411815', '', 'NA', TRUE, FALSE),
('202411811', '', 'NA', TRUE, FALSE),
('202411812', '', 'NA', TRUE, FALSE),
('202412821', '', 'NA', TRUE, FALSE),
('202506866', '', 'NA', TRUE, FALSE),
('202506861', '', 'NA', TRUE, FALSE),
('202512909', 'MBC0000302', 'NA', FALSE, TRUE),
('202411807', '', 'NA', TRUE, FALSE),
('202411808', '', 'NA', TRUE, FALSE),
('202509875', '', 'NA', TRUE, FALSE),
('202408790', '', 'NA', TRUE, FALSE),
('202606935', '', 'NA', TRUE, FALSE),
('202403761', 'MBC0000116', 'NA', FALSE, TRUE),
('202504845', '', 'NA', TRUE, FALSE),
('202511901', '', 'NA', TRUE, FALSE),
('202606936', '', 'NA', TRUE, FALSE),
('202606937', '', 'NA', TRUE, FALSE);

-- Resolve group_head_id by group_code (= CGH_SYS_ID). Skip items whose group failed to seed.
INSERT INTO cst_rm_group_detail
  (group_head_id, item_code, grade_code, is_dummy, is_active, created_by)
SELECT h.group_head_id, s.item_code, s.grade_code, s.is_dummy, s.is_active, 'oracle_csv'
FROM _stg_rmg_item s
JOIN cst_rm_group_head h ON h.group_code = s.group_code AND h.deleted_at IS NULL
-- guard the active-item partial unique index: only insert a real active item_code if no
-- active detail already owns it elsewhere. Empty-code dummies (is_active=FALSE) are unaffected.
WHERE NOT (s.is_active AND s.item_code <> '' AND EXISTS (
  SELECT 1 FROM cst_rm_group_detail d
  WHERE d.item_code = s.item_code AND d.deleted_at IS NULL AND d.is_active = TRUE))
ON CONFLICT DO NOTHING;

DO $$
DECLARE g INTEGER; i INTEGER;
BEGIN
  SELECT count(*) INTO g FROM cst_rm_group_head WHERE created_by='oracle_csv';
  SELECT count(*) INTO i FROM cst_rm_group_detail WHERE created_by='oracle_csv';
  RAISE NOTICE '000460: rm_group heads(oracle_csv)=%  items(oracle_csv)=%  (CSV: 26 heads, 3 real items, 23 empty-dummy items)', g, i;
END $$;

COMMIT;
