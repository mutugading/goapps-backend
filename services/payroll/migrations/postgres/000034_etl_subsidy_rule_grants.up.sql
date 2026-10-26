-- [T009c3] ETL pyr_subsidi_pph → pyr_subsidy_rule (D-30, D-31): payroll_etl menulis rule v1 per (yymm, divisi).
-- pyr_subsidy_rule ber-RLS (000017), jadi policy p_etl dipasang seperti tabel input lain (000028).
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_subsidy_rule TO payroll_etl;
CREATE POLICY p_etl ON payroll.pyr_subsidy_rule TO payroll_etl USING (true) WITH CHECK (true);
