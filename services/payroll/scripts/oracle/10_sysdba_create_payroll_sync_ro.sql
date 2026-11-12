-- 10_sysdba_create_payroll_sync_ro.sql
-- Membuat akun Oracle read-only PAYROLL_SYNC_RO + role PAYROLL_SYNC_ROLE untuk `cmd/sync att` (05§10, D-58 (1)).
-- Hak = persis objek yang dibaca kode sekarang (internal/infrastructure/oracle/att_source.go):
--   S1  MGTHRIS.VIEW_ATT_REKAP_HM
--   S2  MGTHRIS.HM_ATT_ACTUAL + HM_EMP_DATA + HM_DIVISION + HM_SCHEDULE   (HM_SCHEDULE sejak T021e, 05§5)
--   S3  MGTDAT.MGT_PAYROLL_ATT_REKAP_ADD
--   S4  MGTDAT.MGT_PAYROLL_SCH_REKAP
-- Tidak ada hak tulis, tidak ada quota, tidak ada hak sistem selain CREATE SESSION.
--
-- Jalankan sebagai SYSDBA (Oracle 11.2.0.4, service althara). Idempoten: aman dijalankan ulang
-- (jalankan ulang = ganti password + unlock akun).
--   sqlplus /nolog
--   SQL> connect sys@//mgtoradb-scan.mutugading.com:1521/althara as sysdba
--   SQL> @10_sysdba_create_payroll_sync_ro.sql
-- Password diminta interaktif dan tidak disimpan di berkas. Jangan memakai karakter " atau & di password.
-- Sesudahnya: uji sebagai PAYROLL_SYNC_RO (blok "Uji" di akhir berkas), lalu isi .env:
--   PAYROLL_SYNC_ORACLE_HOST/PORT/SERVICE/USER/PASSWORD.
-- Rollback: 99_drop_payroll_sync_ro.sql.

WHENEVER SQLERROR EXIT SQL.SQLCODE ROLLBACK
SET ECHO OFF VERIFY OFF FEEDBACK ON SERVEROUTPUT ON SIZE UNLIMITED
ACCEPT sync_pwd CHAR PROMPT 'Password baru PAYROLL_SYNC_RO: ' HIDE

-- 1. Role
DECLARE
  n NUMBER;
BEGIN
  SELECT COUNT(*) INTO n FROM dba_roles WHERE role = 'PAYROLL_SYNC_ROLE';
  IF n = 0 THEN
    EXECUTE IMMEDIATE 'CREATE ROLE PAYROLL_SYNC_ROLE';
    DBMS_OUTPUT.PUT_LINE('role PAYROLL_SYNC_ROLE dibuat');
  ELSE
    DBMS_OUTPUT.PUT_LINE('role PAYROLL_SYNC_ROLE sudah ada');
  END IF;
END;
/

-- 2. Profile akun service (batch, bukan interaktif). Klien membuka maks 3 koneksi (client.go SetMaxOpenConns).
--    Batas sesi/idle hanya berlaku bila parameter RESOURCE_LIMIT = TRUE; batas password selalu berlaku.
DECLARE
  n NUMBER;
BEGIN
  SELECT COUNT(*) INTO n FROM dba_profiles WHERE profile = 'PAYROLL_SYNC_PROF';
  IF n = 0 THEN
    EXECUTE IMMEDIATE 'CREATE PROFILE PAYROLL_SYNC_PROF LIMIT
      SESSIONS_PER_USER 4 IDLE_TIME 30 CONNECT_TIME 240
      FAILED_LOGIN_ATTEMPTS 5 PASSWORD_LOCK_TIME 1/24 PASSWORD_LIFE_TIME UNLIMITED';
    DBMS_OUTPUT.PUT_LINE('profile PAYROLL_SYNC_PROF dibuat');
  ELSE
    DBMS_OUTPUT.PUT_LINE('profile PAYROLL_SYNC_PROF sudah ada (tidak diubah)');
  END IF;
END;
/

-- 3. User: tablespace default database, tanpa quota (tidak bisa membuat objek).
DECLARE
  n NUMBER;
BEGIN
  SELECT COUNT(*) INTO n FROM dba_users WHERE username = 'PAYROLL_SYNC_RO';
  IF n = 0 THEN
    EXECUTE IMMEDIATE 'CREATE USER PAYROLL_SYNC_RO IDENTIFIED BY "&&sync_pwd" PROFILE PAYROLL_SYNC_PROF ACCOUNT UNLOCK';
    DBMS_OUTPUT.PUT_LINE('user PAYROLL_SYNC_RO dibuat');
  ELSE
    EXECUTE IMMEDIATE 'ALTER USER PAYROLL_SYNC_RO IDENTIFIED BY "&&sync_pwd" PROFILE PAYROLL_SYNC_PROF ACCOUNT UNLOCK';
    DBMS_OUTPUT.PUT_LINE('user PAYROLL_SYNC_RO sudah ada: password diganti, akun di-unlock');
  END IF;
END;
/
UNDEFINE sync_pwd

-- 4. Hak sistem: hanya login, lewat role.
GRANT CREATE SESSION TO PAYROLL_SYNC_ROLE;
GRANT PAYROLL_SYNC_ROLE TO PAYROLL_SYNC_RO;
ALTER USER PAYROLL_SYNC_RO DEFAULT ROLE PAYROLL_SYNC_ROLE;

-- 5. Hak objek: SELECT saja.
GRANT SELECT ON MGTHRIS.HM_ATT_ACTUAL             TO PAYROLL_SYNC_ROLE;  -- S2
GRANT SELECT ON MGTHRIS.HM_EMP_DATA               TO PAYROLL_SYNC_ROLE;  -- S2
GRANT SELECT ON MGTHRIS.HM_DIVISION               TO PAYROLL_SYNC_ROLE;  -- S2
GRANT SELECT ON MGTHRIS.HM_SCHEDULE               TO PAYROLL_SYNC_ROLE;  -- S2 sch_type/ot_inwork (05§5)
GRANT SELECT ON MGTDAT.MGT_PAYROLL_ATT_REKAP_ADD  TO PAYROLL_SYNC_ROLE;  -- S3
GRANT SELECT ON MGTDAT.MGT_PAYROLL_SCH_REKAP      TO PAYROLL_SYNC_ROLE;  -- S4

-- S1 adalah view milik MGTHRIS yang membaca MGTDAT.MGT_PAYROLL_COMPNT_EMP. Grant view ke pihak ketiga
-- butuh MGTHRIS punya SELECT ... WITH GRANT OPTION atas tabel MGTDAT itu; kalau tidak, ORA-01720.
-- Skrip TIDAK menambah hak MGTHRIS diam-diam: bila gagal, keluar dengan pesan dan langkah perbaikannya.
BEGIN
  EXECUTE IMMEDIATE 'GRANT SELECT ON MGTHRIS.VIEW_ATT_REKAP_HM TO PAYROLL_SYNC_ROLE';  -- S1
  DBMS_OUTPUT.PUT_LINE('S1 VIEW_ATT_REKAP_HM: grant OK');
EXCEPTION
  WHEN OTHERS THEN
    IF SQLCODE = -1720 THEN
      RAISE_APPLICATION_ERROR(-20001, 'ORA-01720 pada VIEW_ATT_REKAP_HM. Jalankan dulu (sebagai SYSDBA): '
        || 'GRANT SELECT ON MGTDAT.MGT_PAYROLL_COMPNT_EMP TO MGTHRIS WITH GRANT OPTION; lalu ulangi skrip ini.');
    END IF;
    RAISE;
END;
/

-- 6. Ringkasan hak (harus: 7 SELECT, CREATE SESSION, satu role).
SET LINESIZE 140 PAGESIZE 100
COL grantee FORMAT A20
COL owner FORMAT A10
COL table_name FORMAT A30
COL privilege FORMAT A16
COL granted_role FORMAT A20
SELECT grantee, owner, table_name, privilege FROM dba_tab_privs WHERE grantee IN ('PAYROLL_SYNC_ROLE','PAYROLL_SYNC_RO') ORDER BY 2, 3;
SELECT grantee, privilege FROM dba_sys_privs WHERE grantee IN ('PAYROLL_SYNC_ROLE','PAYROLL_SYNC_RO');
SELECT grantee, granted_role, default_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO';
SELECT username, account_status, profile FROM dba_users WHERE username = 'PAYROLL_SYNC_RO';
SELECT value AS resource_limit FROM v$parameter WHERE name = 'resource_limit';

-- Uji (jalankan manual sesudahnya, login sebagai PAYROLL_SYNC_RO, bukan SYSDBA):
--   connect PAYROLL_SYNC_RO@//mgtoradb-scan.mutugading.com:1521/althara
--   SELECT COUNT(*) FROM mgthris.view_att_rekap_hm         WHERE mpar_yymm = '202610';                 -- S1
--   SELECT COUNT(*) FROM mgthris.hm_att_actual a
--     JOIN mgthris.hm_emp_data e ON e.hmemd_sys_id = a.hata_hmemd_sys_id
--     JOIN mgthris.hm_division dv ON dv.hmdiv_div_id = e.hmemd_hmdiv_div_id
--     LEFT JOIN mgthris.hm_schedule s ON s.hmschd_schedule = a.hata_hmschd_schedule
--    WHERE a.hata_date BETWEEN DATE '2026-09-26' AND DATE '2026-10-25';                                 -- S2
--   SELECT COUNT(*) FROM mgtdat.mgt_payroll_att_rekap_add  WHERE mpara_yymm = '202610';                -- S3
--   SELECT COUNT(*) FROM mgtdat.mgt_payroll_sch_rekap      WHERE mpsr_yymm  = '202610';                -- S4
--   SELECT 1 FROM mgtdat.mgt_payroll_emp WHERE ROWNUM = 1;   -- harus ORA-00942 (di luar S1-S4)
--   SELECT * FROM session_privs;                             -- harus hanya CREATE SESSION
