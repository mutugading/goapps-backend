-- 000462: Seed 23 MB heads present in Oracle CONSUMP/BATCH but missing from mst_mb_head.
-- 15 unique-name heads seed clean; 8 name-collision heads get the '(#<sys_id>)' suffix
-- (Strategy A) to satisfy uix_mst_mb_head_mb_costing. Composition resolves head by
-- oracle_sys_id, never by name — suffix is safe. ADDITIVE ONLY (never DELETE existing).
-- All heads land DRAFT; Layer-B Go job (cmd/backfill-mb-validate) drives them to VALIDATED
-- via the real Submit/Approve/Validate handlers for byte-for-byte auto-gen fidelity.
-- Links every seeded head to the single 'MB' machine (mirrors 000458).
BEGIN;

CREATE TEMP TABLE _stg_mbh (
  oracle_sys_id VARCHAR(30), mb_costing VARCHAR(100), mgt_name VARCHAR(100),
  denier NUMERIC(10,2), filament INTEGER, dozing NUMERIC(10,4),
  check_status VARCHAR(50), mbh_status VARCHAR(100), is_boughtout BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_mbh (oracle_sys_id, mb_costing, mgt_name, denier, filament, dozing, check_status, mbh_status, is_boughtout) VALUES
('20220803368', 'PRE MB-23 FR5-PBT BASE-54.000 PPM (IKV GERMANY) (#20220803368)', 'PRE MB-23 FR5-PBT BASE-54.000 PPM (IKV GERMANY)', 250, 72, NULL, 'Boughtout', 'Boughtout', TRUE),
('20220903402', 'FULL DULL BLACK TBF88 (#20220903402)', 'FULL DULL BLACK TBF88', 500, 96, NULL, 'Boughtout', 'Boughtout', TRUE),
('20221103482', 'POLY. PINK TR-173-MGT A-4429, S.CODE : S3710F (#20221103482)', 'POLY. PINK TR-173-MGT A-4429, S.CODE : S3710F', 250, 48, NULL, 'Boughtout', 'Boughtout', TRUE),
('20231003902', 'ANTIBACTERIAL MASTERBATCH PES968J (#20231003902)', 'ANTIBACTERIAL MASTERBATCH PES968J', 250, 288, NULL, 'Boughtout', 'Boughtout', TRUE),
('20240304021', 'MGT SHIELD BL 5539-D-03246-B (#20240304021)', 'MGT SHIELD BL 5539-D-03246-B', 375, 72, NULL, 'Waiting', 'R and D', FALSE),
('20240404044', 'POLY.PAPILLONRED TR-136-MGT-R, A-4022A, S CODE : S3744F (#20240404044)', 'POLY.PAPILLONRED TR-136-MGT-R, A-4022A, S CODE : S3744F', NULL, NULL, NULL, 'Boughtout', 'Boughtout', TRUE),
('20240604088', 'HIPERSPIN ET MGT-TR-348P IVY GREY 1928 (FETGR 1928, PES-5718E) (#20240604088)', 'HIPERSPIN ET MGT-TR-348P IVY GREY 1928 (FETGR 1928, PES-5718E)', 500, 96, NULL, 'Boughtout', 'Boughtout', TRUE),
('20241004267', 'HIPERSPIN ET MGT-TR-350P ACCORD GREY 1929 (FETGR 1929, PES-5719A) (#20241004267)', 'HIPERSPIN ET MGT-TR-350P ACCORD GREY 1929 (FETGR 1929, PES-5719A)', 250, 48, NULL, 'Boughtout', 'Boughtout', TRUE),
('20260605331', 'MGT HUSK GY 7856 CS-D-05004-B', 'MGT HUSK GY 7856 CS-D-05004-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605332', 'MGT SPRINKLE BL 5893 CS-D-05005-B', 'MGT SPRINKLE BL 5893 CS-D-05005-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605333', 'MGT COBALT BLUE TR-1004 CS-D-05008-B', 'MGT COBALT BLUE TR-1004 CS-D-05008-B', 380, 108, NULL, 'Waiting', 'R and D', FALSE),
('20260605334', 'MGT AT MYSEN BG 6868-D-04953-B', 'MGT AT MYSEN BG 6868-D-04953-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605335', 'MGT AT NARVIK BG 6867-D-04951-B', 'MGT AT NARVIK BG 6867-D-04951-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605336', 'MGT MOLDE BG 6866-D-04947-B', 'MGT MOLDE BG 6866-D-04947-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605337', 'MGT AT IPSWICH BG 6865-D-04950-B', 'MGT AT IPSWICH BG 6865-D-04950-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605338', 'MGT AT SHEFFIELD BG 6864-D-04952-B', 'MGT AT SHEFFIELD BG 6864-D-04952-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605339', 'MGT AT COVENTRY BG 6863-D-04945-B', 'MGT AT COVENTRY BG 6863-D-04945-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605340', 'MGT AT FRANKFURT BG 6870-D-04946-B', 'MGT AT FRANKFURT BG 6870-D-04946-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605341', 'MGT AT BREMEN BG 6871-D-04948-B', 'MGT AT BREMEN BG 6871-D-04948-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605342', 'MGT AT HANNOVER BG 6869-D-04949-B', 'MGT AT HANNOVER BG 6869-D-04949-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260605345', 'MGT EINDHOVEN GN 4375-D-05003-B', 'MGT EINDHOVEN GN 4375-D-05003-B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260705347', 'MGT SPC SUPERFAST PINK E/ 1%--B', 'MGT SPC SUPERFAST PINK E/ 1%--B', 250, 48, NULL, 'Waiting', 'R and D', FALSE),
('20260705348', 'SPC SUPERFAST PINK E/2%--B', 'SPC SUPERFAST PINK E/2%--B', 250, 48, NULL, 'Waiting', 'R and D', FALSE);

INSERT INTO mst_mb_head
  (mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name, mbh_denier, mbh_filament, mbh_dozing,
   mbh_check_status, mbh_status, mbh_is_boughtout, mbh_entry_status, mbh_machine_id, created_by)
SELECT s.oracle_sys_id, s.mb_costing, NULLIF(s.mgt_name,''), s.denier, s.filament, s.dozing,
       NULLIF(s.check_status,''), NULLIF(s.mbh_status,''), s.is_boughtout, 'DRAFT',
       (SELECT mc_id FROM mst_machine WHERE mc_code='MB' AND deleted_at IS NULL),
       'oracle_csv'
FROM _stg_mbh s
ON CONFLICT (mbh_oracle_sys_id) DO NOTHING;

DO $$
DECLARE n INTEGER;
BEGIN
  SELECT count(*) INTO n FROM mst_mb_head WHERE created_by='oracle_csv';
  RAISE NOTICE '000462: mb_head seeded(oracle_csv)=%  (expected up to 23)', n;
END $$;

COMMIT;
