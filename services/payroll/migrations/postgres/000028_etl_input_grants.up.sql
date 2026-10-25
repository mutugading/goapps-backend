-- [T009c] ETL rule & input (D-22): payroll_etl menulis static_value, rule_set/rule_constant, attd_days, subsidi_pph,
-- tax_adj, holiday, addition, abnormal/batal_premi. Policy p_etl hanya dipasang di tabel ber-RLS (pola 000025).
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_static_value, payroll.pyr_rule_set, payroll.pyr_rule_constant,
  payroll.pyr_attd_days, payroll.pyr_subsidi_pph, payroll.pyr_tax_adj, payroll.pyr_holiday, payroll.pyr_addition,
  payroll.pyr_abnormal_premi, payroll.pyr_batal_premi TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_etl;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_static_value','pyr_rule_set','pyr_rule_constant','pyr_attd_days','pyr_subsidi_pph',
    'pyr_tax_adj','pyr_holiday','pyr_addition','pyr_abnormal_premi','pyr_batal_premi'] LOOP
    IF (SELECT relrowsecurity FROM pg_class WHERE oid = ('payroll.' || t)::regclass) THEN
      EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
    END IF;
  END LOOP;
END $$;
