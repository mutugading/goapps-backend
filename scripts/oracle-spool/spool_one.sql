-- Spool satu tabel ke <TABEL>.psv (08§1). Pemakaian: @spool_one.sql <TABEL> ["WHERE ..."]
-- [T027d] Argumen 2 opsional = klausa filter (satu argumen, dikutip), mis. "WHERE VSSV_VS_CODE IN ('PAYROLLSETUP','EMPTAX')".
-- Kolom teks: CR/LF diganti spasi (newline memecah record jadi beberapa baris) dan '|' diganti U+00A6 '¦'
-- (PD_REMARKS PPh21 'Kategori : C | Penghasilan : ...' memuat '|' yang sama dengan pemisah kolom; loader menolaknya).
SET TERMOUT OFF FEEDBACK OFF VERIFY OFF HEADING OFF PAGESIZE 0 LINESIZE 32767 LONG 1000000 TRIMSPOOL ON
-- &2 kosong kalau tidak diberikan (tanpa ini SQL*Plus menanyakan nilainya). &flt = &2 dengan ' digandakan,
-- karena &2 disubstitusi secara teks ke dalam literal string pada SELECT generator di bawah.
COLUMN 2 NEW_VALUE 2 NOPRINT
SELECT '' "2" FROM dual WHERE ROWNUM = 0;
COLUMN flt NEW_VALUE flt NOPRINT
SELECT REPLACE(q'~&2~', '''', '''''') AS flt FROM dual;
SPOOL gen_&1..sql
SELECT 'select '||LISTAGG(CASE WHEN data_type IN ('VARCHAR2','CHAR','NVARCHAR2','NCHAR','CLOB')
         THEN 'REPLACE(REPLACE(REPLACE('||column_name||',CHR(13),'' ''),CHR(10),'' ''),''|'',UNISTR(''\00A6'')) AS '||column_name
         ELSE column_name END, ', ') WITHIN GROUP (ORDER BY column_id)||' from &1 &flt;'
  FROM user_tab_columns WHERE table_name = UPPER('&1');
SPOOL OFF
SET MARKUP CSV ON DELIMITER '|' QUOTE OFF
SET HEADING ON
SPOOL &1..psv
@gen_&1..sql
SPOOL OFF
SET MARKUP CSV OFF
-- bersihkan argumen supaya pemanggilan berikutnya tanpa filter tidak mewarisinya
UNDEFINE 2 flt
