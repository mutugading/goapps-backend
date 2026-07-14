INSERT INTO mst_mb_param (mbp_code, mbp_name, mbp_type, mbp_default_value, mbp_unit, mbp_display_order, mbp_created_by) VALUES
('WASTE','Waste %','SCALAR',2.000,'%',1,'SYSTEM'),
('QUALITY_LOSS','Quality Loss %','SCALAR',0.600,'%',2,'SYSTEM'),
('EFFICIENCY','Efficiency %','SCALAR',94.000,'%',3,'SYSTEM'),
('DEV_EXPENSE','Development Expense %','SCALAR',3.000,'%',4,'SYSTEM'),
('PACKING','Packing Cost','SCALAR',0.100,NULL,5,'SYSTEM'),
('MB_PROD_PER_DAY','MB Production per Day','SCALAR',16.000,'ton',6,'SYSTEM')
ON CONFLICT (mbp_code) DO NOTHING;

INSERT INTO mst_mb_param (mbp_code, mbp_name, mbp_type, mbp_default_option, mbp_unit, mbp_display_order, mbp_created_by) VALUES
('THROUGHPUT_PER_HOUR','Throughput per Hour','PICKLIST','B','kg/hr',7,'SYSTEM'),
('NO_OF_PROCESS','Number of Process','PICKLIST','D',NULL,8,'SYSTEM')
ON CONFLICT (mbp_code) DO NOTHING;

INSERT INTO mst_mb_param_option (mbpo_mbp_code, mbpo_code, mbpo_numeric_value, mbpo_display_order) VALUES
('THROUGHPUT_PER_HOUR','A',30.000,1),
('THROUGHPUT_PER_HOUR','B',40.000,2),
('THROUGHPUT_PER_HOUR','C',50.000,3),
('THROUGHPUT_PER_HOUR','D',60.000,4),
('THROUGHPUT_PER_HOUR','T',70.000,5),
('NO_OF_PROCESS','S',1.000,1),
('NO_OF_PROCESS','D',2.000,2),
('NO_OF_PROCESS','T',3.000,3)
ON CONFLICT (mbpo_mbp_code, mbpo_code) DO NOTHING;

-- mst_parameter rows for the 11 INPUT params + 7 CALCULATED formula-output params
-- (needed before migration 000452 references them via result_param_id/formula_param)
-- VERIFIED against live schema (\d mst_parameter, 2026-07-10): the actual columns are
-- param_code/param_name/data_type/param_category/created_by — NOT param_type/param_created_by
-- as originally assumed. data_type is NOT NULL, CHECK IN ('NUMBER','TEXT','BOOLEAN'); all MB
-- params here are numeric except IS_BOUGHTOUT (BOOLEAN). param_category is the column that
-- takes 'INPUT'/'CALCULATED' (CHECK IN ('INPUT','RATE','CALCULATED','MASTER_LOOKUP')).
-- The unique index on param_code is PARTIAL (idx_mst_parameter_code ... WHERE deleted_at IS NULL),
-- so ON CONFLICT must repeat that WHERE clause to match the index (existing repo convention, see
-- seed_comp_level_group migrations).
-- Addendum 2026-07-11 (design doc §10.2/§10.4): added IS_BOUGHTOUT/MACHINE_MB_FIXED_TOTAL/
-- MB_COMPOSITION_VERSION — referenced by PRD §8.2's worked example and by formulas
-- F_MB_FIXED_COST/F_MB_FINAL_COST, sourced from mst_mb_head's frozen mbh_machine_fixed_total/
-- mbh_current_version columns at auto-gen-on-Validate time (Task 20b). Edited in place, not a
-- new migration, since 000444 has not shipped to any shared environment yet.
-- Addendum 2026-07-11 (Task 20b implementation): these 3 rows are the CAPP/CPP write targets
-- for autogen_handler.go's step 5 (IS_BOUGHTOUT/MACHINE_MB_FIXED_TOTAL/MB_COMPOSITION_VERSION
-- CPP rows), alongside the 8 mbh_param_* rows above (11 total per the design addendum).
INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, created_by) VALUES
('MB_WASTE','MB Waste %','NUMBER','INPUT','SYSTEM'),
('MB_QUALITY_LOSS','MB Quality Loss %','NUMBER','INPUT','SYSTEM'),
('MB_EFFICIENCY','MB Efficiency %','NUMBER','INPUT','SYSTEM'),
('MB_DEV_EXPENSE','MB Dev Expense %','NUMBER','INPUT','SYSTEM'),
('MB_PACKING','MB Packing Cost','NUMBER','INPUT','SYSTEM'),
('MB_PROD_PER_DAY','MB Prod per Day','NUMBER','INPUT','SYSTEM'),
('MB_THROUGHPUT','MB Throughput per Hour','NUMBER','INPUT','SYSTEM'),
('MB_NO_PROCESS','MB Number of Process','NUMBER','INPUT','SYSTEM'),
('IS_BOUGHTOUT','Is Bought-Out','BOOLEAN','INPUT','SYSTEM'),
('MACHINE_MB_FIXED_TOTAL','Machine MB Fixed Total','NUMBER','INPUT','SYSTEM'),
('MB_COMPOSITION_VERSION','MB Composition Version','NUMBER','INPUT','SYSTEM'),
('MB_RM_COST','MB RM Cost','NUMBER','CALCULATED','SYSTEM'),
('MB_WASTE_VAL','MB Waste Value','NUMBER','CALCULATED','SYSTEM'),
('MB_NET_PROD','MB Net Production','NUMBER','CALCULATED','SYSTEM'),
('MB_FIXED_TOTAL','MB Fixed Total','NUMBER','CALCULATED','SYSTEM'),
('MB_COST_OTHERS','MB Cost Others','NUMBER','CALCULATED','SYSTEM'),
('MB_CONV_COST','MB Conversion Cost','NUMBER','CALCULATED','SYSTEM'),
('MB_FINAL_COST','MB Final Cost','NUMBER','CALCULATED','SYSTEM')
ON CONFLICT (param_code) WHERE deleted_at IS NULL DO NOTHING;
