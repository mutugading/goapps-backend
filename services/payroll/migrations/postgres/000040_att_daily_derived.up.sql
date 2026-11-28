-- [T021e] ATT_DERIVED opsi A (05§5, D-58 (2)): S2 membawa tipe jadwal dan flag OT-in-work dari MGTHRIS.HM_SCHEDULE,
-- supaya payroll menghitung sendiri shift (T34/T35) dan otiw_decr (T6). Hasilnya masuk pasr_data snapshot sebagai
-- kunci derived_shift / derived_otiw_decr; MPAR_* tidak diganti dan engine v1 tetap memakai MPAR.
ALTER TABLE payroll_stg.pyr_stg_att_daily ADD COLUMN IF NOT EXISTS psad_sch_type varchar(20);
ALTER TABLE payroll_stg.pyr_stg_att_daily ADD COLUMN IF NOT EXISTS psad_ot_inwork varchar(5);
COMMENT ON COLUMN payroll_stg.pyr_stg_att_daily.psad_sch_type IS 'HM_SCHEDULE.HMSCHD_TYPE (Shift, Paid, ...): shift = hari Shift dengan jam aktual (05§5)';
COMMENT ON COLUMN payroll_stg.pyr_stg_att_daily.psad_ot_inwork IS 'HM_SCHEDULE.HMSCHD_OT_INWORK (Yes/No): otiw_no_days = hari ''No'' (05§5)';
