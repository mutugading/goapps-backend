# Payroll — runbook go-live (initial load production, gladi 202612, go-live 202701)

Bahan T028i/T028j (D-62 hibrida). Hanya perintah dan placeholder: **tanpa** NIK, nama, nominal, atau password.
Password diambil dari password manager (`goapps/<env>/postgres/<role>`), bukan diketik di berkas.
Setiap baris `payroll-sync|report|mailer|recon <sub> -flag` di berkas ini sudah dicocokkan dengan `-h` CLI (T028v).

Placeholder: `<ENV>` (staging|production), `<NS>` (namespace), `<TAG>` (tag image payroll), `<CID>` (company id
MGT di DB target), `<DUMP>` (folder spool lokal IT), `<N_*>` (jumlah baris dari `check_spool.sh`), `<HOST>`.
Keputusan D-68 (ACCEPTED 2026-11-14): production hanya data riil (dump sesudah close Oracle 202612); gladi 202612 di **staging**
dengan absensi dummy (D-64); recon 202701 di **DB salinan** production. `sync dummy-period`/`dummy-janbase` menolak
`APP_ENV=production` (T028w).

## 0. Prasyarat

- Branch payroll sudah di `main` (T028n), image `ghcr.io/mutugading/payroll-service:<TAG>` ada, staging hijau (T028h).
- `<TAG>` sama untuk Deployment dan semua Job CLI (Job membawa `deploy/grants.sql` + `check_ready.sql` dari image).
- `expect_version` = nomor migrasi terakhir di image (`ls services/payroll/migrations/postgres | tail -1`).
- HRIS: karyawan JAKARTA `sts_payroll=1` (D-59 (1), tindakan tim HRIS). Cek sebelum `sync att` JAKARTA.

## 1. Urutan deploy (sekali per environment, `P/docs/database.md` §Urutan)

1. **Secret** (T028s): blok `payroll-db-secret`, `payroll-pii-secret`, `payroll-oracle-sync-secret`,
   `payroll-smtp-secret`, `payroll-report-config` dari `goapps-infra/base/secrets/secrets-template.yaml`.
   `OWNER_USER` **harus `payroll_migrator`** (login; `payroll_owner` NOLOGIN), `OWNER_PASSWORD` = password
   `payroll_migrator` (perbaikan template: T028u2). `APP_PASSWORD` = nilai di userlist `pgbouncer-vps`.
   Kunci PII production dibuat sekali dan disimpan di password manager: kunci hilang = data PII tidak terbaca.
   ```sh
   kubectl get secret -n <NS> payroll-db-secret payroll-pii-secret payroll-oracle-sync-secret payroll-smtp-secret payroll-report-config
   kubectl get secret -n <NS> payroll-db-secret -o jsonpath='{.data.OWNER_USER}' | base64 -d   # harus payroll_migrator
   ```
2. **Bootstrap** (DBA, sekali per cluster): `deploy/bootstrap.sql` sebagai superuser (role, `payroll_db`, schema).
   ```sh
   psql "postgres://<superuser>@<HOST>:5432/postgres?sslmode=require" -v ON_ERROR_STOP=1 -f services/payroll/deploy/bootstrap.sql
   ```
   Lalu set password login role (`payroll_migrator`, `payroll_app`, `payroll_etl`, `payroll_ro`) dari password manager.
3. **Job `payroll-migrate`** (migrate up + grants.sql, satu Job):
   ```sh
   kustomize build services/payroll-service/overlays/<ENV>/jobs | sed "s|IMAGE_TAG|<TAG>|" \
     | yq 'select(.metadata.name == "payroll-migrate")' | kubectl apply -f -
   kubectl wait -n <NS> --for=condition=complete job/payroll-migrate --timeout=10m
   kubectl logs -n <NS> job/payroll-migrate --all-containers
   ```
   Gagal di grants karena `lock_timeout`: hapus Job lalu jalankan ulang (migrate up jadi no-op).
4. **PgBouncer** (kalau `pgbouncer.ini`/userlist berubah, wajib sebelum rollout):
   `kubectl rollout restart deploy/pgbouncer-vps -n database && kubectl rollout status deploy/pgbouncer-vps -n database`
5. **check_ready** (read-only, image membawa `/app/deploy/check_ready.sql`):
   ```sh
   psql "postgres://payroll_ro@<HOST>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 \
        -v expect_version=<N> -f services/payroll/deploy/check_ready.sql > /tmp/check_ready.txt
   grep -c '^FAIL' /tmp/check_ready.txt    # harus 0; selain 0 = berhenti
   ```
6. **Rollout** Deployment lewat ArgoCD (production: sync manual, D-62 (8)), lalu `kubectl rollout status -n <NS> deploy/payroll-service`
   dan `/healthz` sehat.
7. **Data referensi** (sekali, API `payroll.*` admin): `POST /api/v1/payroll/currencies` (IDR),
   `POST /api/v1/payroll/companies` (MGT → catat `<CID>`), `POST /api/v1/payroll/companies/<CID>/divisions` (SOLO, JAKARTA).
   Scope user IT/HR di `pyr_user_scope` diisi admin payroll (D-05/T006a), minimal `<CID>:SOLO`, `<CID>:JAKARTA`.

## 2. Initial load histori (Job `payroll-sync`)

### 2.1 Dump Oracle (IT, read-only, SQL*Plus ≥ 12.2)

**Waktu dump (D-68 (1)(2))**: `salary-legacy` menutup (CLOSED) **setiap** periode yang ada di MAIN/DETAIL dump. Dump
**production** diambil **sesudah close Oracle 202612** (202612 masuk sebagai LEGACY CLOSED; YTD/JAN_BASE 202701 dari LEGACY).
Dump **staging** (gladi) memakai dump yang sudah ada s.d. 202610, sehingga 202611/202612 tetap OPEN untuk engine. Dari folder `<DUMP>`
berisi `spool_one.sql`:
```sh
sqlplus -s <oracle_ro>@<ezconnect> @scripts/oracle-spool/spool_recon_open.sql   # EMP, COMPNT_*, input, MAIN/DETAIL, rekap, THR, DONE
sqlplus -s <oracle_ro>@<ezconnect> @scripts/oracle-spool/spool_t009d2_e2.sql    # + MGT_PAYROLL_SETUP, FM_EXCHANGE_RATE, FM_DEPARTMENT
# IM_VS_STATIC_VALUE (dibaca `inputs`): spool_one.sql IM_VS_STATIC_VALUE (filter VS_CODE PAYROLLSETUP, EMPTAX)
scripts/oracle-spool/check_spool.sh <DUMP> open             # semua OK, tanpa KOLOM-RUSAK; catat baris=<N_*>
for t in MGT_PAYROLL_SETUP FM_EXCHANGE_RATE FM_DEPARTMENT IM_VS_STATIC_VALUE; do echo "$t $(( $(wc -l < <DUMP>/$t.psv) - 1 ))"; done
```
`fx-rate`/`cost-group` membandingkan `-expect-rows` dengan baris **terpilih** (IDR→USD BOOK; ber-`DEPT_FIELD_01`),
bukan seluruh berkas: hitung dulu (`awk -F'|'` pada kolom itu) atau jalankan tanpa flag, lalu cocokkan keluaran.

### 2.2 Urutan load (dependensi di kode, diuji `tests/etl`)

| # | Langkah | Butuh | Bukti di kode/tes |
|---|---|---|---|
| 1 | `migrate` (master + template + legacy_map) | — | `employees`/`gl-setup`/`salary-legacy` memetakan via `pyr_component_legacy_map` (`component.go` "jalankan master dulu") |
| 2 | `employees` (+ COMPNT_EMP, HM_EMP_DATA opsional) | 1 | FK `pec_*`→`pyr_employee`; `TestETL_Employees` |
| 3 | `inputs` (rule_set 1, static value, attd_days, subsidi, tax_adj, holiday, addition, abnormal, batal) | 1 | P000 wajib ada (`input.go`); **sebelum** 5: baris periode CLOSED yang belum dimuat jadi exception (`TestETL_InputsAfterSalaryLegacyClosed`) |
| 4 | `att-legacy` (snapshot LEGACY) | — | **sebelum** 5: `salary-legacy` menolak periode tanpa snapshot LEGACY (`salary_legacy.go`, `TestETL_SalaryLegacyRuns`) |
| 5 | `salary-legacy` (run LEGACY + periode CLOSED) | 1, 4 | `TestETL_SalaryLegacyRuns` |
| 6 | `att-legacy-fail` (PF/BF dari DETAIL) | 4, 5 | usage "setelah salary-legacy"; `TestETL_AttLegacyFailSkipsPeriodWithActiveRun` |
| 7 | `thr-legacy` | 1, 2 | THR head → `pyr_employee`; `TestETL_ThrLegacy` |
| 8 | `gl-setup` | 1 | `TestETL_GlSetup` |
| 9 | `fx-rate` | — | `TestETL_FxRate` |
| 10 | `cost-group` | 2 | UPDATE `pyr_employee.pe_cost_group`; `TestETL_CostGroup` |

`prune-excluded` tidak dipakai (cakupan D-35.1/D-43 sudah di `salary-legacy`). `dummy-*` **dilarang** di staging/production.
Semua langkah idempoten (ulang = no-op/sumber harus sama); setiap langkah satu transaksi, gagal = rollback langkah itu saja.

### 2.3 Menjalankan di cluster

Job `payroll-sync` membawa `PAYROLL_ETL_DSN` + kunci PII; root filesystem read-only, hanya `/out` yang bisa ditulis.
Render dengan `command` diganti skrip yang menunggu dump disalin lalu menjalankan langkah 1–10 berurutan (`set -e`:
berhenti pada error pertama):
```sh
cat > /tmp/initial-load.sh <<'EOF'
set -eu
until [ -f /out/dump/DONE ]; do sleep 5; done
S=/app/payroll-sync; D=/out/dump; C=<CID>
$S migrate         -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-master <N_MASTER> -expect-template <N_TEMPLATE>
$S employees       -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-emp <N_EMP> -expect-compe <N_COMPE>
$S inputs          -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-addition <N_ADDITION> -expect-abnormal <N_ABNORMAL>
$S att-legacy      -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-rekap <N_REKAP> -expect-add <N_REKAP_ADD> -expect-sch <N_SCH>
$S salary-legacy   -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-main <N_MAIN> -expect-detail <N_DETAIL>
$S att-legacy-fail -dir $D -dsn "$PAYROLL_ETL_DSN"
$S thr-legacy      -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-main <N_THR_MAIN> -expect-detail <N_THR_DETAIL>
$S gl-setup        -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-rows <N_SETUP>
$S fx-rate         -dir $D -dsn "$PAYROLL_ETL_DSN" -expect-rows <N_FX_BOOK>
$S cost-group      -dir $D -dsn "$PAYROLL_ETL_DSN" -company $C -expect-rows <N_DEPT_FIELD01>
echo INITIAL-LOAD-OK
EOF
kustomize build services/payroll-service/overlays/<ENV>/jobs | sed "s|IMAGE_TAG|<TAG>|" \
  | yq 'select(.metadata.name == "payroll-sync") | .spec.activeDeadlineSeconds = 14400
        | .spec.template.spec.containers[0].command = ["/bin/sh","-c"]
        | .spec.template.spec.containers[0].args = [load_str("/tmp/initial-load.sh")]' | kubectl apply -f -
POD=$(kubectl get pod -n <NS> -l job-name=payroll-sync -o name | head -1)
kubectl exec -n <NS> $POD -- mkdir -p /out/dump
kubectl cp <DUMP>/. <NS>/${POD#pod/}:/out/dump      # salin semua .psv; DONE ikut, jadi load mulai sesudah cp selesai
kubectl logs -n <NS> -f job/payroll-sync > /tmp/initial-load.log  # simpan lokal; log berisi jumlah & checksum, tanpa NIK
kubectl delete job -n <NS> payroll-sync              # emptyDir (berisi dump gaji) ikut terhapus
```
`DONE` harus tersalin **terakhir**: kalau ragu, salin dulu tanpa DONE lalu `kubectl exec ... touch /out/dump/DONE`.
Kalau ada `EXCEPTION`: jangan pakai `-allow-exceptions` tanpa keputusan user; catat jumlah per tabel dan berhenti.
Langkah gagal → perbaiki penyebab, ulangi seluruh skrip (langkah yang sudah sukses jadi no-op).

## 3. Cek pasca-load (read-only, `payroll_ro`)

```sh
psql "postgres://payroll_ro@<HOST>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 <<'SQL'
SELECT set_config('app.scopes', '<CID>:SOLO,<CID>:JAKARTA', false);
-- (a) D-65: baris MAIN sintetis D-35.2 ber-psh_emp_type numerik. HARUS 0. Skrip d65_emptype.sql TIDAK dijalankan di sini.
SELECT count(*) AS d65_numerik FROM payroll.pyr_salary_head
 WHERE psh_remarks = '[ETL:SYNTHETIC_MAIN D-35.2]' AND psh_emp_type ~ '^[0-9]+$';
-- (b) log ETL terakhir per tabel: src = dst, ok = true; src dibandingkan dengan <N_*> dari check_spool.sh
SELECT DISTINCT ON (pml_table) pml_table, pml_src_rows, pml_dst_rows, pml_ok, pml_run_at
  FROM payroll.pyr_migration_log WHERE pml_company_id = <CID> ORDER BY pml_table, pml_id DESC;
-- (c) periode LEGACY: semua CLOSED dengan run LEGACY ACTIVE (tanpa nominal)
SELECT pp_division, min(pp_yymm), max(pp_yymm), count(*) FILTER (WHERE pp_status <> 'CLOSED') AS bukan_closed
  FROM payroll.pyr_period WHERE pp_company_id = <CID> GROUP BY 1;
SQL
```
Kriteria: (a) = 0; (b) setiap tabel `pml_ok = t` dan `pml_src_rows` = `<N_*>` (tabel yang di-skip D-35.1 tercatat sebagai
SkippedRows di log Job, bukan selisih); (c) `bukan_closed` = 0 dan `max` = 202612. Hasil (agregat) → ACCEPTANCE L.3.

## 4. Gladi end-to-end 202612 (staging, ACCEPTANCE L.2)

**Staging saja, bukan production (D-68 (2))**: initial load 2.3 menutup setiap periode di dump sebagai LEGACY CLOSED,
sedangkan run engine menolak periode CLOSED. Maka staging di-load dari dump s.d. 202610 (bukan dump sesudah close 202612),
dan periode 202611 (bila belum riil) + 202612 dibuat sebagai **dummy** klon 202610 (D-64 amandemen D-68). Hasil gladi
membuktikan alur dan Σ bank = Σ net_pay, **bukan** nominal (D-64 (6)). `sync dummy-*` ditolak di `APP_ENV=production`.
Langkah (staging, namespace `<NS>` staging, `APP_ENV` bukan production):

1. Absensi dummy dari 202610, berurutan 202611 lalu 202612, per divisi (SOLO dan JAKARTA; `-workdays-delta` bila hari kerja beda):
   ```sh
   # args Job payroll-sync (pola header sync-job.yaml); DSN payroll_etl
   ["dummy-period","-dsn","$(PAYROLL_ETL_DSN)","-company","<CID>","-from","202610","-to","202611","-division","SOLO"]
   ["dummy-period","-dsn","$(PAYROLL_ETL_DSN)","-company","<CID>","-from","202611","-to","202612","-division","SOLO"]
   ["dummy-janbase","-dsn","$(PAYROLL_ETL_DSN)","-company","<CID>","-yymm","202701"]   # hanya bila gladi menjangkau Januari (D-64 (4))
   ```
   Klon snapshot + attd_days + addition (tanpa THR); tanpa konek HRIS, jadi tidak ada `att`/`snapshot` di gladi. Ulangi untuk `JAKARTA`.
   Bersihkan dummy sesudah gladi: `dummy-period -purge -owner-dsn "$(PAYROLL_OWNER_DSN)" -to <YYMM> -division <DIV>`.
2. Input periode (addition dsb.) tidak perlu diulang: sudah ikut terklon pada langkah 1.
3. Run lewat API (user ber-permission `payroll.run.dryrun.execute`, `payroll.run.salary.commit`, `payroll.period.salary.approve`):
   ```sh
   curl -sf -X POST "$API/api/v1/payroll/runs" -H "Authorization: Bearer $TOKEN" \
     -d '{"yymm":202612,"division":"SOLO","company_id":<CID>,"mode":"RUN_MODE_DRY"}'          # cek warnings & total
   curl -sf -X POST "$API/api/v1/payroll/runs" -H "Authorization: Bearer $TOKEN" \
     -d '{"yymm":202612,"division":"SOLO","company_id":<CID>,"process_date":"<YYYY-MM-DD>","mode":"RUN_MODE_COMMIT"}'
   curl -sf "$API/api/v1/payroll/runs?yymm=202612&division=SOLO" -H "Authorization: Bearer $TOKEN"   # catat <RUN_ID>
   curl -sf -X POST "$API/api/v1/payroll/periods/202612/SOLO:approve" -H "Authorization: Bearer $TOKEN" -d '{}'
   ```
4. Laporan (Job `payroll-report`, `PAYROLL_APP_DSN` dari Job; keluaran ke `/out`, `kubectl cp` sebelum TTL 1 jam):
   ```sh
   ["recon-komponen","-yymm","202612","-division","SOLO","-company","<CID>","-out","/out/recon_komponen.xlsx"]
   ["reco","-yymm","202612","-division","SOLO","-company","<CID>","-out","/out/reco.xlsx"]
   ["bank","-yymm","202612","-division","SOLO","-company","<CID>","-transfer-date","<YYYY-MM-DD>","-out","/out/bank.xlsx"]
   ["gl","-yymm","202612","-division","SOLO","-company","<CID>","-out","/out/gl.xlsx"]
   ["slip","-yymm","202612","-division","SOLO","-company","<CID>","-out","/out/slip"]
   ```
   Periode belum APPROVED → ditolak; `-allow-open` hanya untuk gladi parsial (tercatat di audit).
   Bukti L.2: Σ bank = Σ net_pay run (dibaca dari workbook, agregat), workbook dibandingkan dengan versi manual HR.
5. Email uji (Job `payroll-mailer`; `-test-to` mengirim semua ke satu alamat uji, status item tidak berubah):
   ```sh
   ["create","-yymm","202612","-division","SOLO","-company","<CID>"]                       # catat batch=<BATCH>
   ["send","-batch","<BATCH>","-yymm","202612","-division","SOLO","-company","<CID>","-test-to","<alamat-uji>","-limit","3"]
   ["cancel","-batch","<BATCH>","-yymm","202612","-division","SOLO","-company","<CID>","-reason","gladi 202612 selesai"]
   ```
6. Ulangi 3–5 untuk JAKARTA. Simpan berkas keluaran di lokasi aman IT (data gaji nyata, tidak dikomit).

## 5. Go-live 202701 (production, T028j) dan rollback

1. Hari proses: absensi **riil** dari HRIS (D-58, D-61: ambil pada hari close legacy, sebelum periode dikunci), per divisi;
   `snapshot` menolak bila staging harian kosong (D-59 (1)):
   ```sh
   ["att","-dsn","$(PAYROLL_ETL_DSN)","-company","<CID>","-yymm","202701","-division","SOLO"]
   ["snapshot","-dsn","$(PAYROLL_ETL_DSN)","-company","<CID>","-yymm","202701","-division","SOLO"]
   ```
   Lalu `inputs` dari dump 202701 OPEN (pola 2.3: `inputs -dir /out/dump -dsn "$PAYROLL_ETL_DSN" -company <CID>`), run dry →
   commit → approve (pola 4.3, `yymm` 202701). Bagian 5.2 (recon) dikerjakan di DB salinan; tanpa dummy.
2. **Recon vs Oracle (syarat bayar D-62 (2b), D-68 (3))** di **DB salinan**, bukan production: `recon diff -target run:<RUN_ID>`
   butuh run LEGACY 202701 ACTIVE dan run ENGINE di DB yang sama, padahal satu periode hanya punya satu run ACTIVE dan
   `salary-legacy` menolak periode yang sudah dibuat engine. Alur: (a) `pg_dump` production → DB recon terpisah (mis.
   `payroll_recon`); (b) di salinan: `att` + `snapshot` + `inputs` 202701 (absensi **riil**, HRIS), run engine dry → commit
   (`<RUN_ID>`); (c) sesudah Oracle menghitung 202701: dump `salary-legacy` 202701 dimuat sebagai LEGACY di salinan;
   (d) `recon diff` di bawah, `PAYROLL_RO_DSN` menunjuk salinan; (e) `DROP DATABASE` salinan sesudah keputusan bayar.
   Production tidak diubah oleh langkah ini. Perintahnya:
   ```sh
   ["diff","-yymm","202701","-division","SOLO","-company","<CID>","-target","run:<RUN_ID>","-out","/out/recon_202701_SOLO.json","-xlsx","/out/recon_202701_SOLO.xlsx"]
   ```
   (Job `payroll-recon`, `PAYROLL_RO_DSN`.) Kriteria: selisih 0 per komponen di luar D-47/D-50, Σ net_pay sama.
3. **Lulus** → laporan + email (pola 4.4–4.5 tanpa `-test-to`):
   ```sh
   ["send","-batch","<BATCH>","-yymm","202701","-division","SOLO","-company","<CID>","-wait"]
   ["report","-batch","<BATCH>","-yymm","202701","-division","SOLO","-company","<CID>","-out","/out/mail_report.xlsx"]
   ```
   Kirim ulang satu orang: `["resend","-batch","<BATCH>","-yymm","202701","-division","SOLO","-company","<CID>","-nik","<NIK>","-reason","<alasan>"]`.
4. **Rollback (gagal salah satu syarat D-62 (2))**: 202701 dibayar dari Oracle seperti biasa. Di GoApps: **jangan**
   jalankan `bank`, `slip`, atau `mailer send`; batch yang sudah dibuat di-`cancel` (pola 4.5). Periode GoApps 202701
   dibiarkan APPROVED/tidak dipakai bayar (reopen untuk run ulang bila perlu, D-68 tidak mengubah ini); hasil recon salinan menjadi bahan task selisih.
   Catat keputusan + tanggal di ACCEPTANCE L.4 (agregat, tanpa NIK), buat task selisih `T028jN`, go-live geser ke 202702
   (ulangi bagian 5 dengan `yymm` 202702).
