-- [T003] rollback 04§3
DROP TYPE IF EXISTS payroll.run_status;
DROP TYPE IF EXISTS payroll.period_status;
DROP TYPE IF EXISTS payroll.comp_type;
DROP DOMAIN IF EXISTS payroll.division;
DROP DOMAIN IF EXISTS payroll.yymm;
DROP DOMAIN IF EXISTS payroll.money_idr;
