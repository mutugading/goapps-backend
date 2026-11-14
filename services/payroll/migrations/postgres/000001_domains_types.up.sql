-- [T003] 04§3 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 3. DOMAIN / TYPE
-- ---------------------------------------------------------------------------
CREATE DOMAIN payroll.money_idr AS numeric(18,2);
CREATE DOMAIN payroll.yymm      AS integer CHECK (VALUE BETWEEN 200001 AND 209912 AND VALUE % 100 BETWEEN 1 AND 12);
CREATE DOMAIN payroll.division  AS varchar(30) CHECK (VALUE IN ('SOLO','JAKARTA'));
CREATE TYPE   payroll.comp_type AS ENUM ('Tunjangan','Potongan');
CREATE TYPE   payroll.period_status AS ENUM ('OPEN','DRAFT','APPROVED','CLOSED');  -- DRAFT≈1, CLOSED≈3
CREATE TYPE   payroll.run_status AS ENUM ('RUNNING','ACTIVE','SUPERSEDED','FAILED','DRY');

