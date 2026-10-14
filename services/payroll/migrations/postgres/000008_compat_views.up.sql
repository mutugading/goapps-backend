-- [T003] 04§10 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 10. VIEW KOMPATIBILITAS (nama kolom legacy, hanya run ACTIVE)
-- ---------------------------------------------------------------------------
CREATE VIEW payroll.v_pyr_mgt_payroll_main WITH (security_barrier) AS
SELECT h.psh_yymm pm_yymm, h.psh_nik pm_nik, h.psh_nama pm_nama, h.psh_dept pm_dept, h.psh_division pm_division,
       h.psh_basic_pay pm_basic_pay, h.psh_tot_tunjangan pm_tot_tunjangan, h.psh_tot_potongan pm_tot_potongan,
       h.psh_rounding pm_rounding, h.psh_remarks pm_remarks,
       CASE p.pp_status WHEN 'CLOSED' THEN 3 ELSE 1 END pm_status
FROM payroll.pyr_salary_head h
JOIN payroll.pyr_period p ON p.pp_yymm = h.psh_yymm AND p.pp_division = h.psh_division AND p.pp_active_run_id = h.psh_run_id;

CREATE VIEW payroll.v_pyr_mgt_payroll_detail WITH (security_barrier) AS
SELECT h.psh_yymm pd_yymm, l.psd_nik pd_nik, h.psh_nama pd_nama, l.psd_id pd_no, l.psd_type::text pd_type,
       l.psd_description pd_description, l.psd_amount pd_amount, l.psd_sts pd_status, l.psd_tax pd_tax,
       l.psd_basis_amount pd_basis_amount, l.psd_remarks pd_remarks, l.psd_tax_paid_company pd_tax_paid_company,
       h.psh_division pd_division
FROM payroll.pyr_salary_detail l
JOIN payroll.pyr_salary_head h ON h.psh_run_id = l.psd_run_id AND h.psh_nik = l.psd_nik
JOIN payroll.pyr_period p ON p.pp_active_run_id = l.psd_run_id;

