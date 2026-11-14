-- =============================================================================
-- Payroll — grant objek (DBA, D-66 C(2), T028n2). Dijalankan OWNER (payroll_migrator, otomatis ROLE
-- payroll_owner) SESUDAH SETIAP `migrate up`. Migrasi tidak memuat GRANT/REVOKE (ADR 0001, lint R4).
--
-- Deklaratif & idempoten: hak tabel/sequence role payroll_app/_etl/_ro di schema payroll + payroll_stg
-- di-reset lalu diberikan ulang ke keadaan akhir (setara hasil migrasi s.d. 000043, bukan urutan sejarahnya).
-- Satu transaksi: aplikasi yang sedang jalan tidak pernah melihat keadaan tanpa grant.
--
-- Hak tingkat cluster/DB/schema (CONNECT, TEMP, USAGE schema, default privileges) ada di bootstrap.sql.
--
-- MENAMBAH TABEL BARU: payroll_ro & payroll_app otomatis dapat SELECT / DML penuh (bagian 2–3); kalau
-- app harus lebih sempit, tambahkan ke daftar pengecualian di bagian 3. payroll_etl TIDAK otomatis dapat
-- apa pun di schema payroll (bagian 4, daftar eksplisit). Fungsi baru: bagian 5. Panduan: docs/database.md.
--
-- Pakai: psql "postgres://payroll_migrator:...@host:5432/payroll_db" -v ON_ERROR_STOP=1 -f deploy/grants.sql
-- Tes testcontainers menjalankan berkas ini lewat database/sql (tanpa meta-command psql).
-- =============================================================================
BEGIN;

-- 0. GUARD: GRANT/REVOKE by a non-owner only WARNs ("no privileges were granted") and psql still exits 0. Without
-- the `SET role = 'payroll_owner'` setting of payroll_migrator (bootstrap.sql) the deploy Job would turn green
-- with no grants at all, so fail loudly instead (T028n5).
DO $$
BEGIN
  IF current_user <> 'payroll_owner' THEN
    RAISE EXCEPTION 'grants.sql must run as payroll_owner (current_user=%): check the role setting of payroll_migrator', current_user;
  END IF;
END $$;

-- 1. RESET: cabut semua hak objek ketiga role (REVOKE di tingkat tabel ikut mencabut grant per kolom).
REVOKE ALL ON ALL TABLES    IN SCHEMA payroll, payroll_stg FROM payroll_app, payroll_etl, payroll_ro;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA payroll, payroll_stg FROM payroll_app, payroll_etl, payroll_ro;

-- 2. payroll_ro: baca semua (RLS tetap membatasi baris).
GRANT SELECT ON ALL TABLES IN SCHEMA payroll, payroll_stg TO payroll_ro;

-- 3. payroll_app: DML penuh di payroll (04§11), staging hanya baca, lalu dipersempit.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payroll TO payroll_app;
GRANT SELECT ON ALL TABLES IN SCHEMA payroll_stg TO payroll_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_app;
-- audit append-only (04§11)
REVOKE UPDATE, DELETE ON payroll.pyr_audit_log FROM payroll_app;
-- rule dikelola migrator/role admin dengan approval (04§11); log ETL & scope user hanya dibaca (000016, 000025)
REVOKE INSERT, UPDATE, DELETE ON payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_rule_component,
  payroll.pyr_migration_log, payroll.pyr_user_scope FROM payroll_app;
-- dokumen/email: template, policy, akun hanya SELECT (D-62); item distribusi tanpa DELETE (jejak audit) (000041)
REVOKE INSERT, UPDATE, DELETE ON payroll.pyr_doc_template, payroll.pyr_slip_password_policy,
  payroll.pyr_mail_account, payroll.pyr_mail_template FROM payroll_app;
REVOKE DELETE ON payroll.pyr_distribution_item FROM payroll_app;
-- view THR hanya baca (000039)
REVOKE INSERT, UPDATE, DELETE ON payroll.v_pyr_thr_main, payroll.v_pyr_thr_detail FROM payroll_app;

-- golang-migrate version table (schema_migrations_payroll in staging/production, schema_migrations in dev) lands in
-- schema payroll and was swept up by the GRANT ON ALL TABLES above: app keeps SELECT only (T028n5). Absent in
-- testcontainers (migrations run as plain SQL), hence the guard.
DO $$
DECLARE t regclass;
BEGIN
  FOREACH t IN ARRAY ARRAY[to_regclass('payroll.schema_migrations_payroll'), to_regclass('payroll.schema_migrations')]
  LOOP
    IF t IS NOT NULL THEN
      EXECUTE format('REVOKE INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON %s FROM payroll_app', t);
    END IF;
  END LOOP;
END $$;

-- 4. payroll_etl (D-22): staging penuh; di payroll hanya yang ditulis/dibaca ETL. Tidak bisa membaca gaji (I10).
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payroll_stg TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll, payroll_stg TO payroll_etl;
-- master & referensi (000025)
GRANT SELECT ON payroll.pyr_company, payroll.pyr_division_ref, payroll.pyr_currency TO payroll_etl;
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_component, payroll.pyr_component_legacy_map,
  payroll.pyr_component_template, payroll.pyr_level_rule TO payroll_etl;
GRANT SELECT, INSERT ON payroll.pyr_migration_log TO payroll_etl;
-- karyawan (000026)
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_employee, payroll.pyr_employee_nik_hist,
  payroll.pyr_employee_component TO payroll_etl;
-- rule & input (000028, 000034, 000042, 000043)
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_static_value, payroll.pyr_rule_set, payroll.pyr_rule_constant,
  payroll.pyr_attd_days, payroll.pyr_subsidi_pph, payroll.pyr_tax_adj, payroll.pyr_holiday, payroll.pyr_addition,
  payroll.pyr_abnormal_premi, payroll.pyr_batal_premi, payroll.pyr_subsidy_rule,
  payroll.pyr_gl_setup, payroll.pyr_gl_group_map, payroll.pyr_fx_rate TO payroll_etl;
-- snapshot absensi LEGACY (000029, 000030)
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_att_snapshot, payroll.pyr_att_snapshot_row TO payroll_etl;
GRANT SELECT (pas_snapshot_id, pas_yymm, pas_division, pas_company_id, pas_locked) ON payroll.pyr_att_snapshot TO payroll_etl;
-- run gaji LEGACY (000029, 000031): INSERT saja di head/detail; SELECT/UPDATE per kolom tanpa nominal
GRANT INSERT ON payroll.pyr_run, payroll.pyr_period, payroll.pyr_salary_head, payroll.pyr_salary_detail TO payroll_etl;
GRANT SELECT (pr_id, pr_company_id, pr_yymm, pr_division, pr_kind, pr_status, pr_snapshot_id, pr_emp_count, pr_line_count)
  ON payroll.pyr_run TO payroll_etl;
GRANT SELECT (pp_yymm, pp_division, pp_company_id, pp_status, pp_active_run_id) ON payroll.pyr_period TO payroll_etl;
GRANT UPDATE (pp_status, pp_active_run_id, closed_by, closed_at) ON payroll.pyr_period TO payroll_etl;
-- audit hanya INSERT aksi ETL_* (000032; policy p_etl_ins)
GRANT INSERT ON payroll.pyr_audit_log TO payroll_etl;
-- THR LEGACY (000039)
GRANT SELECT, INSERT ON payroll.pyr_thr_event TO payroll_etl;
GRANT UPDATE (pte_active_run_id) ON payroll.pyr_thr_event TO payroll_etl;
GRANT INSERT ON payroll.pyr_thr_run, payroll.pyr_thr_head, payroll.pyr_thr_detail TO payroll_etl;
GRANT SELECT (ptr_id, ptr_event_id, ptr_kind, ptr_scheme_id, ptr_rule_set_id, ptr_status, ptr_input_hash, ptr_emp_count,
  ptr_warnings, ptr_started_by, ptr_started_at, ptr_finished_at) ON payroll.pyr_thr_run TO payroll_etl;
GRANT UPDATE (ptr_status, ptr_input_hash, ptr_emp_count, ptr_tot_tunjangan, ptr_tot_potongan, ptr_finished_at)
  ON payroll.pyr_thr_run TO payroll_etl;

-- 5. Fungsi milik payroll: tidak untuk PUBLIC; EXECUTE eksplisit (000007, 000016, 000021, 000024, 000035, 000036, 000039).
REVOKE ALL ON FUNCTION payroll.fn_guard_closed(), payroll.fn_guard_thr_closed(),
  payroll.fn_in_scope(smallint, varchar), payroll.fn_purge_idempotency(interval),
  payroll.fn_component_in_use(bigint), payroll.fn_ytd_out_of_scope(smallint, varchar, int, int, text[], int)
  FROM PUBLIC, payroll_app, payroll_etl, payroll_ro;
GRANT EXECUTE ON FUNCTION payroll.fn_in_scope(smallint, varchar) TO payroll_app, payroll_ro;
GRANT EXECUTE ON FUNCTION payroll.fn_ytd_out_of_scope(smallint, varchar, int, int, text[], int) TO payroll_app, payroll_ro;
GRANT EXECUTE ON FUNCTION payroll.fn_purge_idempotency(interval) TO payroll_app;
GRANT EXECUTE ON FUNCTION payroll.fn_component_in_use(bigint) TO payroll_app;

COMMIT;
