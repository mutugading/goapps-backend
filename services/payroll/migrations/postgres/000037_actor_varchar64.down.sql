-- [T023a] Gagal (22001) bila sudah ada aktor > 30 karakter (UserID UUID). Sengaja tidak dipotong:
-- memotong aktor audit menghilangkan jejak. Ganti nilainya dulu kalau benar-benar perlu turun.
-- p_etl_ins (000032) memakai pal_actor: drop dulu, buat ulang identik setelah ALTER.
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_audit_log;
ALTER TABLE payroll.pyr_employee
  ALTER COLUMN created_by TYPE varchar(30),
  ALTER COLUMN updated_by TYPE varchar(30);
ALTER TABLE payroll.pyr_employee_component
  ALTER COLUMN updated_by TYPE varchar(30);
ALTER TABLE payroll.pyr_employee_component_hist
  ALTER COLUMN updated_by TYPE varchar(30);
ALTER TABLE payroll.pyr_rule_set
  ALTER COLUMN approved_by TYPE varchar(30);
ALTER TABLE payroll.pyr_addition
  ALTER COLUMN created_by TYPE varchar(30);
ALTER TABLE payroll.pyr_att_snapshot
  ALTER COLUMN created_by TYPE varchar(30);
ALTER TABLE payroll.pyr_period
  ALTER COLUMN approved_by TYPE varchar(30),
  ALTER COLUMN closed_by TYPE varchar(30);
ALTER TABLE payroll.pyr_run
  ALTER COLUMN pr_started_by TYPE varchar(30);
ALTER TABLE payroll.pyr_audit_log
  ALTER COLUMN pal_actor TYPE varchar(30);
ALTER TABLE payroll.pyr_fx_rate
  ALTER COLUMN created_by TYPE varchar(30);
ALTER TABLE payroll.pyr_formula_version
  ALTER COLUMN created_by TYPE varchar(30),
  ALTER COLUMN approved_by TYPE varchar(30);
ALTER TABLE payroll.pyr_subsidy_rule
  ALTER COLUMN created_by TYPE varchar(30),
  ALTER COLUMN approved_by TYPE varchar(30);
ALTER TABLE payroll.pyr_subsidy_override
  ALTER COLUMN created_by TYPE varchar(30),
  ALTER COLUMN approved_by TYPE varchar(30);
ALTER TABLE payroll.pyr_gl_export
  ALTER COLUMN pge_generated_by TYPE varchar(30);
ALTER TABLE payroll.pyr_user_scope
  ALTER COLUMN created_by TYPE varchar(30);
CREATE POLICY p_etl_ins ON payroll.pyr_audit_log FOR INSERT TO payroll_etl
  WITH CHECK (pal_actor = 'etl' AND pal_action LIKE 'ETL\_%');
