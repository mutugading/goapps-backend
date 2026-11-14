-- [T009b] ETL karyawan + attachment (D-22): payroll_etl menulis pyr_employee, nik_hist, employee_component
-- dan membaca template untuk menandai baris yang menyimpang (MANUAL). Policy p_etl mengikuti pola 000025.

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_employee','pyr_employee_nik_hist','pyr_employee_component'] LOOP
    EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
  END LOOP;
END $$;
