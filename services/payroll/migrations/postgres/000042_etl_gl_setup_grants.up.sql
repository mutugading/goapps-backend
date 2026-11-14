-- [T028k] ETL MGT_PAYROLL_SETUP → pyr_gl_setup + pyr_gl_group_map (11§4.5): payroll_etl menulis setup GL.
-- Kedua tabel ber-RLS (000016), jadi policy p_etl dipasang seperti tabel input lain (000028).
DROP POLICY IF EXISTS p_etl ON payroll.pyr_gl_setup;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_gl_group_map;
CREATE POLICY p_etl ON payroll.pyr_gl_setup TO payroll_etl USING (true) WITH CHECK (true);
CREATE POLICY p_etl ON payroll.pyr_gl_group_map TO payroll_etl USING (true) WITH CHECK (true);
