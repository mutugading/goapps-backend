DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_addition','pyr_subsidy_rule','pyr_rule_constant','pyr_gl_setup','pyr_attd_days','pyr_tax_adj',
    'pyr_subsidi_pph','pyr_abnormal_premi','pyr_gl_group_map'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_etl_sel ON payroll.%I', t);
    EXECUTE format('DROP POLICY IF EXISTS p_etl_ins ON payroll.%I', t);
    EXECUTE format('DROP POLICY IF EXISTS p_etl_upd ON payroll.%I', t);
    EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
  END LOOP;
END $$;

-- Policy app di tabel yang punya p_ro kembali dipakai bersama app+ro (keadaan 000048).
DO $$
DECLARE p record;
BEGIN
  FOR p IN
    SELECT n.nspname sch, c.relname tbl, pl.polname pol
    FROM pg_policy pl JOIN pg_class c ON c.oid = pl.polrelid JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname IN ('payroll', 'payroll_stg') AND pl.polname <> 'p_ro' AND pl.polpermissive
      AND pl.polname IN ('p_scope', 'p_parent', 'p_self') AND pl.polroles = ARRAY['payroll_app'::regrole::oid]
      AND EXISTS (SELECT 1 FROM pg_policy x WHERE x.polrelid = pl.polrelid AND x.polname = 'p_ro')
  LOOP
    EXECUTE format('ALTER POLICY %I ON %I.%I TO payroll_app, payroll_ro', p.pol, p.sch, p.tbl);
  END LOOP;
  FOR p IN
    SELECT n.nspname sch, c.relname tbl FROM pg_policy pl JOIN pg_class c ON c.oid = pl.polrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname IN ('payroll', 'payroll_stg') AND pl.polname = 'p_ro'
  LOOP
    EXECUTE format('DROP POLICY p_ro ON %I.%I', p.sch, p.tbl);
  END LOOP;
END $$;
