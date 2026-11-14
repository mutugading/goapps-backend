-- [T020b] I11 (09§6.3, 05§4 langkah 2, 04§11): sync wajib membaca status periode & lock snapshot sebelum menulis
-- staging. Grant SELECT per kolom (hanya yang dipakai cek lock); tabel lain, termasuk pyr_salary_detail (I10), tetap
-- tertutup. USAGE schema payroll sudah ada sejak 000025. Policy p_etl hanya SELECT, hanya bila tabel ber-RLS.

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_period','pyr_att_snapshot'] LOOP
    IF NOT (SELECT relrowsecurity FROM pg_class WHERE oid = ('payroll.' || t)::regclass) THEN
      RAISE EXCEPTION 'payroll.% tidak ber-RLS; p_etl_lock tidak bisa dipasang', t;
    END IF;
    EXECUTE format('DROP POLICY IF EXISTS p_etl_lock ON payroll.%I', t);
    EXECUTE format('CREATE POLICY p_etl_lock ON payroll.%I FOR SELECT TO payroll_etl USING (true)', t);
  END LOOP;
END $$;
