-- =============================================================================
-- Payroll — cek kesiapan DB sesudah deploy (READ-ONLY, T028t, D-66 C). Dijalankan sebagai payroll_recon atau owner:
--
--   psql "postgres://payroll_recon:...@host:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 \
--        -v expect_version=49 [-v expect_policies=248] -f deploy/check_ready.sql | grep -v '^OK'
--
-- Keluaran: satu baris `OK|FAIL <cek> <detail>` per cek, tanpa nama karyawan/nominal (I25; hanya nama objek DB).
-- psql keluar 0 walau ada FAIL: pemanggil membaca baris FAIL (mis. `grep -c '^FAIL'`).
--   expect_version   WAJIB: versi migrasi terakhir di image (tanpa ini cek `version` FAIL).
--   expect_policies  opsional: jumlah policy RLS yang diharapkan (tanpa ini hanya diperiksa tiap tabel ber-RLS punya policy).
--
-- Hak efektif dibandingkan lewat has_*_privilege (bukan teks ACL) dengan matriks yang SAMA dengan deploy/grants.sql
-- (bagian 2-5). Mengubah grants.sql = ubah matriks di sini (tes TestCheckReady menjaga keduanya sejalan).
-- Tanpa tabel sementara/DDL: payroll_recon (read-only, tanpa TEMP) cukup.
-- =============================================================================
\set QUIET on
\pset format unaligned
\pset tuples_only on
\pset footer off
\if :{?expect_version}
\else
  \set expect_version ''
\endif
\if :{?expect_policies}
\else
  \set expect_policies ''
\endif

WITH
-- ---- versi migrasi (golang-migrate: schema_migrations_payroll di staging/production, schema_migrations di dev) ----
ver AS (
  SELECT t.tbl,
         (xpath('/row/version/text()', q.x))[1]::text AS version,
         (xpath('/row/dirty/text()',   q.x))[1]::text AS dirty
  FROM (SELECT coalesce(to_regclass('payroll.schema_migrations_payroll'), to_regclass('payroll.schema_migrations')) AS tbl) t
  LEFT JOIN LATERAL (
    SELECT query_to_xml(format('SELECT version, dirty FROM %s', t.tbl), false, true, '') AS x WHERE t.tbl IS NOT NULL
  ) q ON true
),
-- ---- objek ----
objs AS (
  SELECT c.oid, n.nspname AS sch, c.relname AS rel, c.relkind, c.relrowsecurity AS rls
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname IN ('payroll', 'payroll_stg') AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
),
privs(code, name) AS (VALUES ('S','SELECT'), ('I','INSERT'), ('U','UPDATE'), ('D','DELETE'), ('T','TRUNCATE')),
-- payroll_app: DML penuh di payroll, kecuali tabel ini (huruf = hak yang DICABUT; grants.sql bagian 3)
app_deny(rel, privs) AS (VALUES
  ('pyr_audit_log', 'UD'),
  ('pyr_rule_set', 'IUD'), ('pyr_rule_constant', 'IUD'), ('pyr_rule_component', 'IUD'),
  ('pyr_migration_log', 'IUD'), ('pyr_user_scope', 'IUD'),
  ('pyr_doc_template', 'IUD'), ('pyr_slip_password_policy', 'IUD'), ('pyr_mail_account', 'IUD'), ('pyr_mail_template', 'IUD'),
  ('pyr_distribution_item', 'D'),
  ('pyr_formula_version', 'IUD'), ('pyr_formula_dependency', 'IUD'), ('pyr_formula_test_case', 'IUD'), ('pyr_subsidy_rule', 'IUD'), ('pyr_subsidy_override', 'IUD'),
  ('v_pyr_thr_main', 'IUD'), ('v_pyr_thr_detail', 'IUD'),
  ('schema_migrations_payroll', 'IUD'), ('schema_migrations', 'IUD')
),
-- payroll_etl di schema payroll: hanya daftar ini, hak tingkat tabel (grants.sql bagian 4); payroll_stg: DML penuh
etl_allow(rel, privs) AS (VALUES
  ('pyr_company', 'S'), ('pyr_division_ref', 'S'), ('pyr_currency', 'S'),
  ('pyr_component', 'SIU'), ('pyr_component_legacy_map', 'SIU'), ('pyr_component_template', 'SIU'), ('pyr_component_template_owner', 'S'), ('pyr_level_rule', 'SIU'),
  ('pyr_migration_log', 'SI'),
  ('pyr_employee', 'SIU'), ('pyr_employee_nik_hist', 'SIU'), ('pyr_employee_component', 'SIU'),
  ('pyr_static_value', 'SIU'), ('pyr_rule_set', 'SIU'), ('pyr_rule_constant', 'SIU'), ('pyr_attd_days', 'SIU'),
  ('pyr_subsidi_pph', 'SIU'), ('pyr_tax_adj', 'SIU'), ('pyr_holiday', 'SIU'), ('pyr_addition', 'SIU'),
  ('pyr_abnormal_premi', 'SIU'), ('pyr_batal_premi', 'SIU'), ('pyr_subsidy_rule', 'SIU'),
  ('pyr_gl_setup', 'SIU'), ('pyr_gl_group_map', 'SIU'), ('pyr_fx_rate', 'SIU'),
  ('pyr_att_snapshot', 'SIU'), ('pyr_att_snapshot_row', 'SIUD'),
  ('pyr_run', 'I'), ('pyr_period', 'I'), ('pyr_salary_head', 'I'), ('pyr_salary_detail', 'I'),
  ('pyr_audit_log', 'I'),
  ('pyr_thr_event', 'SI'), ('pyr_thr_run', 'I'), ('pyr_thr_head', 'I'), ('pyr_thr_detail', 'I')
),
-- payroll_etl: tabel yang hanya punya hak per kolom (tidak ada hak tingkat tabel; mencakup larangan baca nominal)
etl_cols(rel, priv) AS (VALUES
  ('pyr_run', 'SELECT'), ('pyr_period', 'SELECT'), ('pyr_period', 'UPDATE'),
  ('pyr_thr_run', 'SELECT'), ('pyr_thr_run', 'UPDATE'), ('pyr_thr_event', 'UPDATE')
),
-- ---- ACL tabel/view ----
acl AS (
  SELECT r.role, o.sch || '.' || o.rel AS obj, p.name AS priv,
         has_table_privilege(r.role, o.oid, p.name) AS actual,
         CASE r.role
           WHEN 'payroll_ro'  THEN p.code = 'S'
           WHEN 'payroll_app' THEN CASE
             WHEN p.code = 'T' THEN false
             WHEN o.sch = 'payroll_stg' THEN p.code = 'S'
             ELSE position(p.code IN coalesce(d.privs, '')) = 0 AND p.code <> 'T' END
           WHEN 'payroll_etl' THEN CASE
             WHEN p.code = 'T' THEN false
             WHEN o.sch = 'payroll_stg' THEN true
             ELSE position(p.code IN coalesce(e.privs, '')) > 0 END
         END AS expected
  FROM objs o
  CROSS JOIN (VALUES ('payroll_app'), ('payroll_etl'), ('payroll_ro')) r(role)
  CROSS JOIN privs p
  LEFT JOIN app_deny  d ON d.rel = o.rel
  LEFT JOIN etl_allow e ON e.rel = o.rel AND o.sch = 'payroll'
),
acl_line AS (
  SELECT role, count(*) AS n,
         count(*) FILTER (WHERE actual IS DISTINCT FROM expected) AS bad,
         left(string_agg(obj || ':' || priv || '=' || actual, ',' ORDER BY obj, priv)
              FILTER (WHERE actual IS DISTINCT FROM expected), 300) AS detail
  FROM acl GROUP BY role
),
acl_cols AS (
  SELECT count(*) AS n,
         count(*) FILTER (WHERE NOT has_any_column_privilege('payroll_etl', 'payroll.' || rel, priv)) AS bad,
         left(string_agg(rel || ':' || priv, ',') FILTER (WHERE NOT has_any_column_privilege('payroll_etl', 'payroll.' || rel, priv)), 300) AS detail
  FROM etl_cols
),
-- sequence: app USAGE di payroll; etl USAGE di payroll + payroll_stg; ro tidak ada
seq AS (
  SELECT r.role, c.relname,
         has_sequence_privilege(r.role, c.oid, 'USAGE') AS actual,
         CASE r.role WHEN 'payroll_app' THEN n.nspname = 'payroll' WHEN 'payroll_etl' THEN true ELSE false END AS expected
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  CROSS JOIN (VALUES ('payroll_app'), ('payroll_etl'), ('payroll_ro')) r(role)
  WHERE n.nspname IN ('payroll', 'payroll_stg') AND c.relkind = 'S'
),
seq_line AS (
  SELECT count(*) AS n, count(*) FILTER (WHERE actual IS DISTINCT FROM expected) AS bad,
         left(string_agg(role || ':' || relname, ',') FILTER (WHERE actual IS DISTINCT FROM expected), 300) AS detail
  FROM seq
),
-- fungsi milik payroll (grants.sql bagian 5): siapa yang boleh EXECUTE (PUBLIC termasuk dalam has_function_privilege)
fexp(fname, allowed) AS (VALUES
  ('fn_guard_closed', ''), ('fn_guard_thr_closed', ''),
  ('fn_in_scope', 'payroll_app,payroll_ro'), ('fn_ytd_out_of_scope', 'payroll_app,payroll_ro'),
  ('fn_purge_idempotency', 'payroll_app'), ('fn_component_in_use', 'payroll_app')
),
fn AS (
  SELECT f.fname, r.role, p.oid AS poid,
         CASE WHEN p.oid IS NOT NULL THEN has_function_privilege(r.role, p.oid, 'EXECUTE') END AS actual,
         r.role = ANY (string_to_array(f.allowed, ',')) AS expected
  FROM fexp f
  CROSS JOIN (VALUES ('payroll_app'), ('payroll_etl'), ('payroll_ro')) r(role)
  LEFT JOIN pg_proc p ON p.proname = f.fname AND p.pronamespace = 'payroll'::regnamespace
),
fn_line AS (
  SELECT count(*) AS n, count(*) FILTER (WHERE actual IS DISTINCT FROM expected) AS bad,
         left(string_agg(fname || ':' || role, ',') FILTER (WHERE actual IS DISTINCT FROM expected), 300) AS detail
  FROM fn
),
-- ---- RLS ----
global_tables(rel) AS (VALUES
  ('payroll.pyr_batal_premi'), ('payroll.pyr_company'), ('payroll.pyr_component_master'), ('payroll.pyr_currency'),
  ('payroll.pyr_division_ref'), ('payroll.pyr_fx_rate'), ('payroll.pyr_holiday'), ('payroll.pyr_level_rule'),
  ('payroll.pyr_rule_set'), ('payroll.pyr_static_value'),
  -- tabel versi golang-migrate (tanpa data karyawan; app hanya SELECT, lihat grants.sql)
  ('payroll.schema_migrations_payroll'), ('payroll.schema_migrations')
),
rls_line AS (
  SELECT count(*) FILTER (WHERE o.relkind IN ('r', 'p') AND NOT o.rls AND g.rel IS NULL) AS no_rls,
         left(string_agg(o.sch || '.' || o.rel, ',') FILTER (WHERE o.relkind IN ('r', 'p') AND NOT o.rls AND g.rel IS NULL), 300) AS no_rls_detail,
         count(*) FILTER (WHERE o.rls AND NOT EXISTS (SELECT 1 FROM pg_policy pl WHERE pl.polrelid = o.oid)) AS no_policy,
         left(string_agg(o.sch || '.' || o.rel, ',') FILTER (WHERE o.rls AND NOT EXISTS (SELECT 1 FROM pg_policy pl WHERE pl.polrelid = o.oid)), 300) AS no_policy_detail,
         (SELECT count(*) FROM pg_policy pl JOIN objs x ON x.oid = pl.polrelid) AS policies
  FROM objs o LEFT JOIN global_tables g ON g.rel = o.sch || '.' || o.rel
),
-- ---- SECURITY DEFINER wajib pin search_path = payroll, pg_catalog (D-66 C(4)) ----
secdef AS (
  SELECT count(*) AS n,
         count(*) FILTER (WHERE NOT coalesce(p.proconfig @> ARRAY['search_path=payroll, pg_catalog'], false)) AS bad,
         left(string_agg(p.proname, ',') FILTER (WHERE NOT coalesce(p.proconfig @> ARRAY['search_path=payroll, pg_catalog'], false)), 300) AS detail,
         (SELECT count(*) FROM unnest(ARRAY['fn_guard_closed', 'fn_guard_thr_closed', 'fn_ytd_out_of_scope',
                                           'fn_purge_idempotency', 'fn_component_in_use']) w(name)
           WHERE NOT EXISTS (SELECT 1 FROM pg_proc q WHERE q.proname = w.name AND q.pronamespace = 'payroll'::regnamespace AND q.prosecdef)) AS missing
  FROM pg_proc p WHERE p.pronamespace IN ('payroll'::regnamespace, 'payroll_stg'::regnamespace) AND p.prosecdef
),
-- ---- role ----
rl AS (
  SELECT v.role, r.oid IS NOT NULL AS ok_exists,
         coalesce(r.rolsuper OR r.rolbypassrls, false) AS risky,
         v.role IN ('payroll_owner', 'payroll_ro') AND coalesce(r.rolcanlogin, false) AS owner_login,
         -- payroll_recon (login baca, T033g2) wajib anggota payroll_ro dan tidak memegang role lain
         coalesce(v.role = 'payroll_recon' AND r.oid IS NOT NULL AND (NOT pg_has_role(r.oid, 'payroll_ro', 'USAGE')
           OR EXISTS (SELECT 1 FROM pg_auth_members m WHERE m.member = r.oid AND m.roleid <> 'payroll_ro'::regrole)), false) AS bad_member
  FROM (VALUES ('payroll_owner'), ('payroll_app'), ('payroll_etl'), ('payroll_ro'), ('payroll_recon')) v(role)
  LEFT JOIN pg_roles r ON r.rolname = v.role
),
results(ord, status, name, detail) AS (
  -- versi
  SELECT 10, CASE WHEN tbl IS NULL THEN 'FAIL'
                  WHEN nullif(:'expect_version', '') IS NULL THEN 'FAIL'
                  WHEN version = :'expect_version' THEN 'OK' ELSE 'FAIL' END,
         'version',
         CASE WHEN tbl IS NULL THEN 'tabel versi golang-migrate tidak ditemukan'
              WHEN nullif(:'expect_version', '') IS NULL THEN 'expect_version tidak diberikan (actual=' || coalesce(version, '?') || ')'
              WHEN version = :'expect_version' THEN version
              ELSE 'actual=' || coalesce(version, '?') || ' expect=' || :'expect_version' END
  FROM ver
  UNION ALL
  SELECT 11, CASE WHEN dirty = 'false' THEN 'OK' ELSE 'FAIL' END, 'dirty', 'dirty=' || coalesce(dirty, '?') FROM ver
  UNION ALL
  -- role
  SELECT 20, CASE WHEN ok_exists AND NOT risky AND NOT owner_login AND NOT bad_member THEN 'OK' ELSE 'FAIL' END, 'role ' || role,
         CASE WHEN NOT ok_exists THEN 'tidak ada' WHEN risky THEN 'superuser/bypassrls' WHEN owner_login THEN 'role grup bisa login'
              WHEN bad_member THEN 'keanggotaan bukan tepat payroll_ro'
              ELSE 'ada' END
  FROM rl
  UNION ALL
  -- ACL
  SELECT 30, CASE WHEN bad = 0 AND n > 0 THEN 'OK' ELSE 'FAIL' END, 'acl_tabel ' || role,
         CASE WHEN n = 0 THEN 'tidak ada objek' WHEN bad = 0 THEN n || ' hak cocok' ELSE bad || ' selisih ' || detail END
  FROM acl_line
  UNION ALL
  SELECT 31, CASE WHEN bad = 0 THEN 'OK' ELSE 'FAIL' END, 'acl_kolom payroll_etl',
         CASE WHEN bad = 0 THEN n || ' hak cocok' ELSE bad || ' hilang ' || detail END
  FROM acl_cols
  UNION ALL
  SELECT 32, CASE WHEN bad = 0 THEN 'OK' ELSE 'FAIL' END, 'acl_sequence',
         CASE WHEN bad = 0 THEN n || ' hak cocok' ELSE bad || ' selisih ' || detail END
  FROM seq_line
  UNION ALL
  SELECT 33, CASE WHEN bad = 0 THEN 'OK' ELSE 'FAIL' END, 'acl_fungsi',
         CASE WHEN bad = 0 THEN n || ' hak cocok' ELSE bad || ' selisih ' || detail END
  FROM fn_line
  UNION ALL
  -- RLS
  SELECT 40, CASE WHEN no_rls = 0 THEN 'OK' ELSE 'FAIL' END, 'rls_tabel',
         CASE WHEN no_rls = 0 THEN 'semua tabel non-global ber-RLS' ELSE no_rls || ' tanpa RLS ' || no_rls_detail END
  FROM rls_line
  UNION ALL
  SELECT 41, CASE WHEN no_policy = 0 AND policies > 0 AND (nullif(:'expect_policies', '') IS NULL OR policies = nullif(:'expect_policies', '')::bigint)
                  THEN 'OK' ELSE 'FAIL' END,
         'rls_policy',
         CASE WHEN no_policy > 0 THEN no_policy || ' tabel ber-RLS tanpa policy ' || no_policy_detail
              WHEN nullif(:'expect_policies', '') IS NOT NULL AND policies <> nullif(:'expect_policies', '')::bigint
                THEN 'actual=' || policies || ' expect=' || :'expect_policies'
              WHEN policies = 0 THEN 'tidak ada policy'
              ELSE policies || ' policy' END
  FROM rls_line
  UNION ALL
  -- SECURITY DEFINER
  SELECT 50, CASE WHEN bad = 0 AND missing = 0 THEN 'OK' ELSE 'FAIL' END, 'secdef_search_path',
         CASE WHEN missing > 0 THEN missing || ' fungsi SECURITY DEFINER wajib tidak ada'
              WHEN bad > 0 THEN bad || ' tanpa search_path=payroll,pg_catalog ' || detail
              ELSE n || ' fungsi, search_path terkunci' END
  FROM secdef
)
SELECT status || ' ' || name || ' ' || detail FROM results ORDER BY ord, name;
