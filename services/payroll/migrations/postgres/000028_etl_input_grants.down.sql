DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_static_value','pyr_rule_set','pyr_rule_constant','pyr_attd_days','pyr_subsidi_pph',
    'pyr_tax_adj','pyr_holiday','pyr_addition','pyr_abnormal_premi','pyr_batal_premi'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_etl ON payroll.%I', t);
  END LOOP;
END $$;
REVOKE ALL ON payroll.pyr_static_value, payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_attd_days,
  payroll.pyr_subsidi_pph, payroll.pyr_tax_adj, payroll.pyr_holiday, payroll.pyr_addition,
  payroll.pyr_abnormal_premi, payroll.pyr_batal_premi FROM payroll_etl;
