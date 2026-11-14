-- 11_sysdba_grant_sync_delta.sql
-- Untuk PAYROLL_SYNC_RO yang SUDAH ADA: hanya menambah hak SELECT yang kurang. Tidak membuat user,
-- tidak mengganti password, tidak mengubah profile, tidak mencabut hak apa pun.
-- Hak target = objek yang dibaca kode sync (internal/infrastructure/oracle/att_source.go):
--   S1  MGTHRIS.VIEW_ATT_REKAP_HM
--   S2  MGTHRIS.HM_ATT_ACTUAL, HM_EMP_DATA, HM_DIVISION, HM_SCHEDULE   (HM_SCHEDULE sejak T021e, 05§5)
--   S3  MGTDAT.MGT_PAYROLL_ATT_REKAP_ADD
--   S4  MGTDAT.MGT_PAYROLL_SCH_REKAP
-- Grant diberikan ke role PAYROLL_SYNC_ROLE bila role itu ada dan sudah dimiliki user; selain itu langsung ke user.
-- Idempoten. Jalankan sebagai SYSDBA:
--   SQL> connect sys@//mgtoradb-scan.mutugading.com:1521/althara as sysdba
--   SQL> @11_sysdba_grant_sync_delta.sql

WHENEVER SQLERROR EXIT SQL.SQLCODE ROLLBACK
SET ECHO OFF VERIFY OFF FEEDBACK OFF SERVEROUTPUT ON SIZE UNLIMITED
SET LINESIZE 140 PAGESIZE 100
COL grantee FORMAT A20
COL owner FORMAT A10
COL table_name FORMAT A30
COL privilege FORMAT A16
COL granted_role FORMAT A20

PROMPT === Keadaan SEBELUM ===
SELECT username, account_status, profile FROM dba_users WHERE username = 'PAYROLL_SYNC_RO';
SELECT grantee, granted_role, default_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO';
SELECT grantee, owner, table_name, privilege FROM dba_tab_privs
 WHERE grantee = 'PAYROLL_SYNC_RO'
    OR grantee IN (SELECT granted_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO')
 ORDER BY 1, 2, 3;

DECLARE
  TYPE t_obj IS TABLE OF VARCHAR2(80);
  objs    t_obj := t_obj('MGTHRIS.HM_ATT_ACTUAL', 'MGTHRIS.HM_EMP_DATA', 'MGTHRIS.HM_DIVISION', 'MGTHRIS.HM_SCHEDULE',
                         'MGTDAT.MGT_PAYROLL_ATT_REKAP_ADD', 'MGTDAT.MGT_PAYROLL_SCH_REKAP',
                         'MGTHRIS.VIEW_ATT_REKAP_HM');
  n       NUMBER;
  grantee VARCHAR2(30);
  own     VARCHAR2(30);
  tab     VARCHAR2(50);
BEGIN
  SELECT COUNT(*) INTO n FROM dba_users WHERE username = 'PAYROLL_SYNC_RO';
  IF n = 0 THEN
    RAISE_APPLICATION_ERROR(-20002, 'User PAYROLL_SYNC_RO tidak ada: pakai 10_sysdba_create_payroll_sync_ro.sql');
  END IF;

  SELECT COUNT(*) INTO n FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO' AND granted_role = 'PAYROLL_SYNC_ROLE';
  grantee := CASE WHEN n > 0 THEN 'PAYROLL_SYNC_ROLE' ELSE 'PAYROLL_SYNC_RO' END;
  DBMS_OUTPUT.PUT_LINE('grant ke: ' || grantee);

  SELECT COUNT(*) INTO n FROM dba_sys_privs WHERE privilege = 'CREATE SESSION'
     AND grantee IN ('PAYROLL_SYNC_RO', 'PAYROLL_SYNC_ROLE');
  IF n = 0 THEN
    EXECUTE IMMEDIATE 'GRANT CREATE SESSION TO ' || grantee;
    DBMS_OUTPUT.PUT_LINE('+ CREATE SESSION');
  END IF;

  FOR i IN 1 .. objs.COUNT LOOP
    own := SUBSTR(objs(i), 1, INSTR(objs(i), '.') - 1);
    tab := SUBSTR(objs(i), INSTR(objs(i), '.') + 1);
    SELECT COUNT(*) INTO n FROM dba_tab_privs
     WHERE owner = own AND table_name = tab AND privilege = 'SELECT'
       AND grantee IN ('PAYROLL_SYNC_RO', 'PAYROLL_SYNC_ROLE');
    IF n > 0 THEN
      DBMS_OUTPUT.PUT_LINE('= ' || objs(i) || ' sudah ada');
    ELSE
      BEGIN
        EXECUTE IMMEDIATE 'GRANT SELECT ON ' || objs(i) || ' TO ' || grantee;
        DBMS_OUTPUT.PUT_LINE('+ SELECT ' || objs(i));
      EXCEPTION
        WHEN OTHERS THEN
          IF SQLCODE = -1720 THEN  -- view S1 membaca MGTDAT.MGT_PAYROLL_COMPNT_EMP
            RAISE_APPLICATION_ERROR(-20001, 'ORA-01720 pada ' || objs(i) || '. Jalankan dulu: '
              || 'GRANT SELECT ON MGTDAT.MGT_PAYROLL_COMPNT_EMP TO MGTHRIS WITH GRANT OPTION; lalu ulangi skrip ini.');
          END IF;
          RAISE;
      END;
    END IF;
  END LOOP;
END;
/

PROMPT === Keadaan SESUDAH (harus 7 objek di atas, SELECT saja) ===
SELECT grantee, owner, table_name, privilege FROM dba_tab_privs
 WHERE grantee = 'PAYROLL_SYNC_RO'
    OR grantee IN (SELECT granted_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO')
 ORDER BY 1, 2, 3;
PROMPT === Hak sistem dan role: sebaiknya hanya CREATE SESSION, hak lain diperiksa manual ===
SELECT grantee, privilege FROM dba_sys_privs
 WHERE grantee = 'PAYROLL_SYNC_RO'
    OR grantee IN (SELECT granted_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO');
SELECT grantee, granted_role FROM dba_role_privs WHERE grantee = 'PAYROLL_SYNC_RO';
