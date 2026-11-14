-- Spool dump production untuk T009d2 (absensi) dan T009e2 (MAIN/DETAIL), format 08§1:
-- <TABEL>.psv, pemisah '|', baris pertama = nama kolom, tanpa quote, tanpa newline di dalam sel.
-- Jalankan di folder berisi spool_one.sql (berkas ditulis ke folder kerja). Butuh SQL*Plus >= 12.2
-- (Instant Client 23; klien Oracle 8 tidak bisa, lihat memori oracle-sqlplus-env). Read-only.
-- COMPNT_EMP dipakai att-legacy-fail (T009e3).
ALTER SESSION SET NLS_NUMERIC_CHARACTERS='.,';
ALTER SESSION SET NLS_DATE_FORMAT='YYYY-MM-DD HH24:MI:SS';
-- NUMWIDTH 40: default 10 memotong angka (kurs FM_EXCHANGE_RATE .000055561729 → .00005556; angka besar jadi 1.2E+13).
SET NUMWIDTH 40
@spool_one.sql MGT_PAYROLL_ATT_REKAP
@spool_one.sql MGT_PAYROLL_ATT_REKAP_ADD
@spool_one.sql MGT_PAYROLL_SCH_REKAP
@spool_one.sql MGT_PAYROLL_MAIN
@spool_one.sql MGT_PAYROLL_DETAIL
@spool_one.sql MGT_PAYROLL_COMPNT_EMP
@spool_one.sql MGT_PAYROLL_SETUP
-- [T028p] kurs IDR→USD BOOK untuk jurnal GL (sync fx-rate, D-63)
@spool_one.sql FM_EXCHANGE_RATE
-- [T028q] master dept: DEPT_FIELD_01 = cost group MGT|MB|CONTRACT (sync cost-group; spool_one gagal ORA-01489)
@spool_dept.sql
EXIT
