-- [T009b] ETL karyawan + attachment (D-22): payroll_etl menulis pyr_employee, nik_hist, employee_component
-- dan membaca template untuk menandai baris yang menyimpang (MANUAL). Policy p_etl mengikuti pola 000025.
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_employee, payroll.pyr_employee_nik_hist, payroll.pyr_employee_component TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_etl;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_employee','pyr_employee_nik_hist','pyr_employee_component'] LOOP
    EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
  END LOOP;
END $$;
