-- [T033g2] Review PR #283 #2 (D-77 (3)): RLS role baca & ETL dipersempit.
--
-- (1) payroll_ro = recon/cek IT lintas divisi DAN company (filter company eksplisit di query) (login payroll_recon ∈ payroll_ro, bootstrap). Policy bersama
--     app+ro (p_scope/p_parent/p_self) memakai GUC app.scopes yang bisa di-SET sendiri oleh sesi ro, jadi bagi ro
--     scope bukan batas keamanan. Policy itu kini khusus payroll_app; ro mendapat p_ro FOR SELECT USING (true) di
--     tabel yang sama. GUC tetap boleh diset ro sebagai filter tampilan; query recon memfilter company/divisi
--     secara eksplisit (loader YTD memakai fn_in_scope, T033g2). Tabel tanpa policy ro sebelumnya (mis.
--     pyr_idempotency_key) tetap tertutup bagi ro.
-- (2) p_etl USING (true) di tabel rule/input/subsidi/GL dipecah per perintah (tanpa DELETE): SELECT tetap penuh
--     (ON CONFLICT, cek frozen), INSERT/UPDATE hanya baris sumber ETL:
--       pyr_addition       pa_source = 'LEGACY' (atau baris gladi DUMMY-D64, D-64)
--       pyr_subsidy_rule   created_by = 'etl' (rule app/DRAFT/approval tidak tersentuh)
--       pyr_rule_constant  rule set legacy prc_rule_set_id = 1
--       pyr_gl_setup       pgs_legacy_row IS NOT NULL (baris hasil MGT_PAYROLL_SETUP)
--     Upsert ETL ke baris milik app (mis. addition MANUAL dengan kunci sama) kini ditolak RLS dan menggagalkan load:
--     disengaja (ETL tidak menimpa input app), lihat T033g5.
--     Tabel tanpa penanda sumber (attd_days, tax_adj, subsidi_pph, abnormal_premi, gl_group_map) belum punya
--     penulis app; ETL tetap boleh INSERT/UPDATE, tanpa DELETE. Periode CLOSED dijaga fn_guard_closed.
--     Audit ETL sudah sempit sejak 000037 (p_etl_ins: actor 'etl', aksi ETL_*).

-- (1) ro keluar dari policy bersama, dapat p_ro.
DO $$
DECLARE p record;
BEGIN
  FOR p IN
    SELECT n.nspname sch, c.relname tbl, pl.polname pol
    FROM pg_policy pl JOIN pg_class c ON c.oid = pl.polrelid JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname IN ('payroll', 'payroll_stg')
      AND pl.polroles @> ARRAY['payroll_app'::regrole::oid, 'payroll_ro'::regrole::oid]
      AND cardinality(pl.polroles) = 2
  LOOP
    EXECUTE format('ALTER POLICY %I ON %I.%I TO payroll_app', p.pol, p.sch, p.tbl);
    IF NOT EXISTS (SELECT 1 FROM pg_policy x WHERE x.polrelid = format('%I.%I', p.sch, p.tbl)::regclass AND x.polname = 'p_ro') THEN
      EXECUTE format('CREATE POLICY p_ro ON %I.%I FOR SELECT TO payroll_ro USING (true)', p.sch, p.tbl);
    END IF;
  END LOOP;
  IF EXISTS (SELECT 1 FROM pg_policy pl JOIN pg_class c ON c.oid = pl.polrelid JOIN pg_namespace n ON n.oid = c.relnamespace
             WHERE n.nspname IN ('payroll', 'payroll_stg') AND 'payroll_ro'::regrole::oid = ANY (pl.polroles) AND pl.polname <> 'p_ro') THEN
    RAISE EXCEPTION 'masih ada policy ber-GUC untuk payroll_ro';
  END IF;
END $$;

-- (2) p_etl dipecah per perintah.
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_addition',       $c$pa_source = 'LEGACY' OR created_by = 'DUMMY-D64'$c$),
    ('pyr_subsidy_rule',   $c$created_by = 'etl'$c$),
    ('pyr_rule_constant',  $c$prc_rule_set_id = 1$c$),
    ('pyr_gl_setup',       $c$pgs_legacy_row IS NOT NULL$c$),
    ('pyr_attd_days',      'true'),
    ('pyr_tax_adj',        'true'),
    ('pyr_subsidi_pph',    'true'),
    ('pyr_abnormal_premi', 'true'),
    ('pyr_gl_group_map',   'true')
  ) AS v(tbl, cond) LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_etl ON payroll.%I', t.tbl);
    EXECUTE format('DROP POLICY IF EXISTS p_etl_sel ON payroll.%I', t.tbl);
    EXECUTE format('DROP POLICY IF EXISTS p_etl_ins ON payroll.%I', t.tbl);
    EXECUTE format('DROP POLICY IF EXISTS p_etl_upd ON payroll.%I', t.tbl);
    EXECUTE format('CREATE POLICY p_etl_sel ON payroll.%I FOR SELECT TO payroll_etl USING (true)', t.tbl);
    EXECUTE format('CREATE POLICY p_etl_ins ON payroll.%I FOR INSERT TO payroll_etl WITH CHECK (%s)', t.tbl, t.cond);
    EXECUTE format('CREATE POLICY p_etl_upd ON payroll.%I FOR UPDATE TO payroll_etl USING (%s) WITH CHECK (%s)', t.tbl, t.cond, t.cond);
  END LOOP;
END $$;
