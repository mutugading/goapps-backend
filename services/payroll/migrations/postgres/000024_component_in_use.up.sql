-- [T008c] Apakah komponen sudah dipakai? Attachment karyawan (semua divisi) atau detail run di periode CLOSED.
-- RLS membuat pemanggil hanya melihat divisinya, padahal komponen berlaku se-company, jadi dicek lewat fungsi
-- SECURITY DEFINER yang hanya mengembalikan angka/boolean untuk satu sysid.
CREATE OR REPLACE FUNCTION payroll.fn_component_in_use(p_sysid bigint, OUT attachments bigint, OUT closed_run boolean)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
BEGIN
  SELECT count(*) INTO attachments FROM payroll.pyr_employee_component WHERE pec_component_sysid = p_sysid;
  SELECT EXISTS (SELECT 1 FROM payroll.pyr_salary_detail d
                   JOIN payroll.pyr_run r ON r.pr_id = d.psd_run_id
                   JOIN payroll.pyr_period p ON p.pp_company_id = r.pr_company_id AND p.pp_yymm = r.pr_yymm AND p.pp_division = r.pr_division
                  WHERE d.psd_component_sysid = p_sysid AND p.pp_status = 'CLOSED') INTO closed_run;
END $$;
REVOKE ALL ON FUNCTION payroll.fn_component_in_use(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION payroll.fn_component_in_use(bigint) TO payroll_app;
