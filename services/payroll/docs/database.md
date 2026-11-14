# Payroll — database, migrasi & grant (DevOps)

Keputusan: D-66 opsi C (hibrida terhadap ADR 0001 *one database per service*).

| | Nilai |
|---|---|
| Database | `payroll_db` (dulu `payroll`) |
| Tabel versi golang-migrate | `schema_migrations_payroll` (staging/production, overlay infra) |
| Schema | `payroll`, `payroll_stg` (dibuat `deploy/bootstrap.sql`, bukan migrasi) |
| Role | `payroll_owner` (NOLOGIN, pemilik objek), `payroll_migrator` (login migrasi, otomatis `ROLE payroll_owner`), `payroll_app`, `payroll_etl`, `payroll_ro` |

## Urutan

1. **Sekali per cluster**: `deploy/bootstrap.sql` sebagai superuser/DBA (role, DB, schema, CONNECT/TEMP, USAGE schema,
   default privileges, event trigger). Dev: `goapps/deploy/payroll/init.sh` (salinan identik, dijaga `tests/drift`).
2. **Setiap deploy**: `migrate up` sebagai `payroll_migrator`.
3. **Setiap deploy, sesudah langkah 2**: `deploy/grants.sql` sebagai `payroll_migrator`:
   ```sh
   psql "postgres://payroll_migrator:...@<host>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 -q -f deploy/grants.sql
   ```
   Migrasi tidak memuat GRANT/REVOKE (ADR 0001, Migration Lint R4). Tanpa langkah ini tabel baru tidak bisa
   diakses `payroll_app`/`payroll_ro`, dan hak `payroll_etl` tidak terpasang. Skrip idempoten, berjalan dalam satu transaksi,
   dan mengembalikan hak akses ke keadaan yang dideklarasikan (hak tambahan yang diberikan manual akan dicabut).

Dev lokal: service `payroll` di `goapps/docker-compose.yml` menjalankan langkah 2 dan 3 saat start; `make migrate-up`
juga menjalankan `make grants`. Testcontainers (`tests/{e2e,etl,isolation}`) menjalankan bootstrap → migrasi → grants.

## Menambah objek baru

- Tabel baru di `payroll`: `payroll_ro` mendapat SELECT dan `payroll_app` mendapat DML otomatis. Kalau hak app harus lebih
  sempit, tambahkan REVOKE di bagian 3 `grants.sql`. `payroll_etl` tidak mendapat apa pun kecuali didaftarkan di bagian 4.
- Fungsi baru: REVOKE dari PUBLIC dan GRANT EXECUTE di bagian 5.
- Perubahan grant = ubah `grants.sql` (bukan migrasi baru), lalu cek tes isolasi.
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
