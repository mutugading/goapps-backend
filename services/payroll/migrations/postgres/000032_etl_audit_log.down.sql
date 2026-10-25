DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_audit_log;
REVOKE USAGE ON SEQUENCE payroll.pyr_audit_log_pal_id_seq FROM payroll_etl;
REVOKE INSERT ON payroll.pyr_audit_log FROM payroll_etl;
