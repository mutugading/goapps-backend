# Payroll — database, migrasi & grant (DevOps)

Keputusan: D-66 opsi C (hibrida terhadap ADR 0001 *one database per service*).

| | Nilai |
|---|---|
| Database | `payroll_db` (dulu `payroll`) |
| Tabel versi golang-migrate | `schema_migrations_payroll` (staging/production, overlay infra) |
| Schema | `payroll`, `payroll_stg` (dibuat `deploy/bootstrap.sql`, bukan migrasi) |
| Role | `payroll_owner` (NOLOGIN, pemilik objek), `payroll_migrator` (login migrasi, otomatis `ROLE payroll_owner`), `payroll_app`, `payroll_etl`, `payroll_ro` (NOLOGIN, grup baca), `payroll_recon` (login baca, anggota `payroll_ro`) |

## Urutan

1. **Sekali per cluster**: `deploy/bootstrap.sql` sebagai superuser/DBA (role, DB, schema, CONNECT/TEMP, USAGE schema,
   default privileges, event trigger). Dev: `goapps/deploy/payroll/init.sh` (salinan identik, dijaga `tests/drift`).
2. **Setiap deploy**: `migrate up` sebagai `payroll_migrator`.
3. **Setiap deploy, sesudah langkah 2**: `deploy/grants.sql` sebagai `payroll_migrator`:
   ```sh
   psql "postgres://payroll_migrator:...@<host>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 -q -f deploy/grants.sql
   ```
   Migrasi tidak memuat GRANT/REVOKE (ADR 0001, Migration Lint R4). Tanpa langkah ini tabel baru tidak bisa
   diakses `payroll_app`/`payroll_ro` (grup login `payroll_recon`), dan hak `payroll_etl` tidak terpasang. Skrip idempoten, berjalan dalam satu transaksi,
   dan mengembalikan hak akses ke keadaan yang dideklarasikan (hak tambahan yang diberikan manual akan dicabut).

Staging/production: langkah 2 dan 3 dijalankan **satu Job** `payroll-migrate` (goapps-infra
`services/payroll-service/base/jobs/migrate-job.yaml`): initContainer image payroll menjalankan `migrate up` lalu
menyalin `/app/deploy/grants.sql` (dibawa image, `Dockerfile`) ke emptyDir; container `postgres:18-alpine` menjalankan
`psql -f grants.sql` dengan login owner yang sama. Image runtime sengaja tanpa `psql`. Job selesai hanya kalau keduanya
berhasil; kalau grants gagal, migrasi sudah terpasang dan aplikasi tetap memakai grant lama: perbaiki lalu jalankan
ulang Job (`migrate up` jadi no-op). Urutan deploy: bootstrap (DBA, sekali) → Job `payroll-migrate` → restart `pgbouncer-vps` (kalau `pgbouncer.ini` berubah) → rollout Deployment.
Kalau grants gagal karena `lock_timeout` (10 s, ada transaksi app yang menahan lock), cukup jalankan ulang Job.
`grants.sql` menolak jalan kalau `current_user` bukan `payroll_owner` (setting role `payroll_migrator` hilang):
tanpa guard ini GRANT oleh non-owner hanya WARNING dan Job tetap hijau tanpa grant.

PgBouncer (`pgbouncer-vps`) memakai kunci `payroll_db` (kunci lama `payroll` dihapus). ConfigMap-nya tanpa hash
suffix, jadi setelah sync **wajib** `kubectl rollout restart deploy/pgbouncer-vps -n database` (atau `RELOAD` di
console admin) **sebelum** rollout payroll-service; kalau tidak, pod gagal dengan `no such database: payroll_db`.
Job CLI (sync/recon/report/mailer) konek langsung ke `payroll_db` (bukan lewat PgBouncer).

`grants.sql` juga mencabut DML `payroll_app` pada tabel versi golang-migrate (`schema_migrations_payroll` /
`schema_migrations`, ada di schema `payroll`), sehingga app hanya bisa SELECT.

Dev lokal: service `payroll` di `goapps/docker-compose.yml` menjalankan langkah 2 dan 3 saat start; `make migrate-up`
juga menjalankan `make grants`. Testcontainers (`tests/{e2e,etl,isolation}`) menjalankan bootstrap → migrasi → grants.

4. **Setiap deploy, sesudah langkah 3 (read-only)**: `deploy/check_ready.sql` sebagai `payroll_recon` atau owner (image membawa
   `/app/deploy/check_ready.sql`):
   ```sh
   psql "postgres://payroll_recon:...@<host>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 \
        -v expect_version=<migrasi terakhir di image> [-v expect_policies=<jumlah policy>] -f deploy/check_ready.sql | grep -v '^OK'
   ```
   Keluaran satu baris `OK|FAIL <cek> <detail>` per cek (versi `schema_migrations_payroll` + `dirty=false`, role, ACL
   tabel/kolom/sequence/fungsi lewat `has_*_privilege`, tabel non-global ber-RLS + policy, `SECURITY DEFINER` ber-`search_path`),
   tanpa nama/nominal (I25). psql keluar 0 walau ada FAIL: gagalkan langkah deploy kalau `grep -c '^FAIL'` > 0. Matriks hak
   di berkas ini harus sejalan dengan `grants.sql`; ubah `grants.sql` = ubah `check_ready.sql` (tes `TestCheckReady`).

## Role baca & RLS (T033g2, D-77 (3))

- `payroll_ro` = grup baca **lintas divisi dan company** (termasuk `pyr_user_scope` & `pyr_audit_log` semua company) untuk recon/cek IT (`cmd/recon`, `check_ready.sql`, runbook). Login-nya
  `payroll_recon` (INHERIT, `default_transaction_read_only`, CONNECTION LIMIT 5); DSN `PAYROLL_RECON_DSN`
  (`PAYROLL_RO_DSN` masih dibaca sebagai fallback sampai secret infra pindah, T033g3).
- Policy ro = `p_ro FOR SELECT USING (true)` di setiap tabel ber-RLS yang dibaca ro. `app.scopes` **bukan** batas keamanan
  bagi ro (sesi bisa men-SET sendiri), jadi tidak dipakai di policy ro; `cmd/recon` tetap men-SET-nya sebagai filter, dan
  setiap query ro wajib memfilter company/divisi eksplisit (loader YTD: `fn_in_scope`). Tabel tanpa `p_ro`
  (mis. `pyr_idempotency_key`) tertutup bagi ro.
- Policy `p_scope`/`p_parent`/`p_self` khusus `payroll_app`: bagi app, scope tetap batas keamanan (diset server dari
  `pyr_user_scope`, bukan dari klien).
- `payroll_etl` di tabel rule/input/subsidi/GL: `p_etl_sel` (baca), `p_etl_ins`/`p_etl_upd` hanya baris sumber ETL
  (`pa_source='LEGACY'`, `psr created_by='etl'`, `prc_rule_set_id=1`, `pgs_legacy_row` terisi), tanpa DELETE. Sumber yang
  bentrok dengan baris milik app (addition MANUAL/UPLOAD/LOAN, rule subsidi bernama sama bukan `etl`, setup GL tanpa
  `pgs_legacy_row`) dicatat sebagai Exception per baris, tidak menimpa dan tidak menggagalkan load (T033g5).
- `payroll_app` tidak menulis `pyr_formula_version`, `pyr_formula_dependency`, `pyr_formula_test_case`, `pyr_subsidy_rule`, `pyr_subsidy_override` (grants.sql bagian 3)
  sampai `fn_activate_formula/_subsidy` SECURITY DEFINER (4-eyes) dibuat bersama UI formula Fase 2.
- Baris `pyr_audit_log` sebelum allowlist (T033g1): jalankan sekali `scripts/fix/t033g2_audit_scrub.sql` sebagai
  `payroll_migrator` di tiap DB (idempoten, TIDAK bisa dibatalkan: ambil dulu
  `pg_dump -t payroll.pyr_audit_log` sebagai cadangan; allowlist dijaga sama dengan `mask.go` oleh tes).

## Menambah objek baru

- Tabel baru ber-RLS yang dibaca recon: tambahkan `p_ro` (migrasi) bersama policy app-nya.
- Tabel baru di `payroll`: `payroll_ro` mendapat SELECT dan `payroll_app` mendapat DML otomatis. Kalau hak app harus lebih
  sempit, tambahkan REVOKE di bagian 3 `grants.sql`. `payroll_etl` tidak mendapat apa pun kecuali didaftarkan di bagian 4.
- Fungsi baru: REVOKE dari PUBLIC dan GRANT EXECUTE di bagian 5.
- Perubahan grant = ubah `grants.sql` (bukan migrasi baru) **dan** matriks di `check_ready.sql`, lalu cek tes isolasi.
  Tabel global tanpa RLS didaftarkan di `check_ready.sql` (`global_tables`) dan `globalTables` di tes I20.
- Rollback (`migrate down`): down migrasi tidak lagi mencabut grant, dan `grants.sql` merujuk objek sampai migrasi
  terakhir. Sesudah down, jalankan `grants.sql` dari versi kode yang sama dengan skema (kalau tidak, skrip gagal utuh
  karena satu transaksi; tidak ada grant parsial).

## DB lama bernama `payroll` (rename, satu kali)

DB dev/staging yang dibuat sebelum T028n2 tidak dimigrasi ulang. Grant objeknya sudah sesuai dengan hasil `grants.sql`
(dibuktikan selisih ACL 0). Yang perlu diganti hanya nama DB:

```sql
-- superuser, tanpa koneksi aktif ke DB payroll (stop service payroll dulu)
ALTER DATABASE payroll RENAME TO payroll_db;
```

Grant DB (CONNECT/TEMP) dan `ALTER ROLE payroll_migrator IN DATABASE ... SET role` ikut pindah (melekat ke OID DB).
Sesudah rename, jalankan `deploy/grants.sql` sekali. Untuk production (initial load T028i), DB dibuat baru
langsung dengan nama `payroll_db` lewat bootstrap, jadi rename tidak diperlukan.

Dev stack dengan volume `payroll-pg` lama: bootstrap hanya jalan saat data dir kosong, dan healthcheck compose kini
memakai `-d payroll_db`. Lakukan rename di atas (atau hapus volume) sebelum `docker compose up`, kalau tidak
`payroll-postgres` unhealthy dan service `payroll` tidak start.
