-- [T003] rollback 04§7
DROP TABLE IF EXISTS payroll.pyr_att_snapshot_row, payroll.pyr_att_snapshot CASCADE;
DROP TABLE IF EXISTS payroll_stg.pyr_stg_sch_rekap, payroll_stg.pyr_stg_att_rekap_add, payroll_stg.pyr_stg_att_daily, payroll_stg.pyr_stg_att_rekap_hm, payroll_stg.pyr_stg_sync_batch CASCADE;
