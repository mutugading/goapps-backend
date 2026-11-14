-- [T028r] D-65 (C): perbaikan SATU KALI khusus dev — psh_emp_type baris MAIN sintetis D-35.2 di run LEGACY CLOSED.
-- MAIN sintetis lama menyimpan PE_STS_EMP ('2') di psh_emp_type; yang benar = nama level (pe_level, kunci
-- pyr_gl_group_map). ETL sudah benar sejak T028q, jadi hanya DB yang dimuat sebelum T028q yang perlu skrip ini.
-- Production diisi baru (T028i) dan TIDAK boleh disentuh; runbook T028i memeriksa 0 baris emp_type numerik.
--
-- Jalankan sebagai owner (payroll_migrator), mis.:
--   PGPASSWORD=... psql -h localhost -p 5437 -U payroll_migrator -d payroll_db -v CONFIRM_DEV=yes -f scripts/fix/d65_emptype.sql
-- Satu transaksi; guard CLOSED (trg_guard_header) dimatikan hanya di dalamnya. Idempoten (run kedua = 0 baris).
-- Hanya menyentuh: run LEGACY, psh_remarks = penanda sintetis, psh_emp_type numerik, pe_level terisi.
-- Audit: satu baris pyr_audit_log per periode (jumlah baris), tanpa NIK.

\set ON_ERROR_STOP on
\if :{?CONFIRM_DEV}
\else
  \set CONFIRM_DEV no
\endif

-- Penjaga (allowlist, apa pun nama DB-nya): wajib `-v CONFIRM_DEV=yes` DAN host lokal (kosong, soket unix, localhost,
-- 127.0.0.1, ::1). Tunnel/port-forward ke production tampak 'localhost': konfirmasi eksplisit adalah pengaman terakhir,
-- jangan diberikan untuk tunnel ke production.
SELECT ((:'HOST' = '' OR :'HOST' LIKE '/%' OR :'HOST' IN ('localhost', '127.0.0.1', '::1')) AND :'CONFIRM_DEV' = 'yes') AS allowed \gset
\if :allowed
\else
  -- \quit tidak memberi kode keluar; exception + ON_ERROR_STOP menghentikan psql dengan kode 3.
  DO $$ BEGIN RAISE EXCEPTION 'D-65: DITOLAK, skrip hanya untuk DB dev: host harus lokal dan -v CONFIRM_DEV=yes wajib'; END $$;
\endif

BEGIN;

ALTER TABLE payroll.pyr_salary_head DISABLE TRIGGER trg_guard_header;

WITH upd AS (
  UPDATE payroll.pyr_salary_head h
     SET psh_emp_type = btrim(e.pe_level)
    FROM payroll.pyr_run r, payroll.pyr_employee e
   WHERE r.pr_id = h.psh_run_id
     AND r.pr_kind = 'LEGACY'
     AND h.psh_remarks = '[ETL:SYNTHETIC_MAIN D-35.2]'
     AND h.psh_emp_type ~ '^[0-9]+$'
     AND e.pe_company_id = h.psh_company_id AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
     AND btrim(coalesce(e.pe_level, '')) <> '' AND e.pe_level !~ '^[0-9]+$'
  RETURNING h.psh_company_id, h.psh_yymm, h.psh_division
), ins AS (
  INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_company_id, pal_after)
  SELECT 'script:d65_emptype', 'D65_EMPTYPE_FIX', 'pyr_salary_head', psh_yymm || '/' || psh_division, psh_division, psh_company_id,
         jsonb_build_object('rows', count(*), 'decision', 'D-65 C')
    FROM upd GROUP BY psh_company_id, psh_yymm, psh_division
  RETURNING pal_entity_key, pal_after->>'rows' AS rows
)
SELECT pal_entity_key AS periode, rows AS baris_diperbaiki FROM ins ORDER BY 1;

ALTER TABLE payroll.pyr_salary_head ENABLE TRIGGER trg_guard_header;

-- Laporan sisa (tidak diubah). sisa_sintetis_numerik = baris ber-penanda sintetis D-35.2 yang tak bisa diperbaiki (tanpa
-- pe_level / NIK berubah): runbook T028i harus 0. sisa_lain_numerik = baris LEGACY lain ber-emp_type numerik (tanpa penanda,
-- mis. dimuat sebelum T009e10): di luar cakupan D-65, hanya diinformasikan.
SELECT count(*) FILTER (WHERE h.psh_remarks = '[ETL:SYNTHETIC_MAIN D-35.2]') AS sisa_sintetis_numerik,
       count(*) FILTER (WHERE h.psh_remarks IS DISTINCT FROM '[ETL:SYNTHETIC_MAIN D-35.2]') AS sisa_lain_numerik
  FROM payroll.pyr_salary_head h JOIN payroll.pyr_run r ON r.pr_id = h.psh_run_id
 WHERE r.pr_kind = 'LEGACY' AND h.psh_emp_type ~ '^[0-9]+$';

COMMIT;
