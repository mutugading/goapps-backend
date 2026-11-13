-- SALINAN untuk CI/tes dari goapps/deploy/payroll/bootstrap.sql (T028h2): tests/** membacanya dari repo ini.
-- Ubah KEDUANYA bersamaan (stack dev workspace memakai berkas di goapps/deploy/payroll/).
-- =============================================================================
-- Payroll — bootstrap cluster-level (D-04). Dijalankan SUPERUSER, sekali.
-- Sumber: spec 04 §0–2 (+ §13 hanya sebagai acuan; pg_hba tidak dipakai di dev).
-- Variabel psql: pw_app, pw_etl, pw_ro, pw_mig (lihat init.sh).
-- Migrasi aplikasi (T003+) berjalan sebagai payroll_migrator, otomatis ber-ROLE
-- payroll_owner (ALTER ROLE ... IN DATABASE ... SET role), jadi golang-migrate
-- tidak perlu SET ROLE manual.
-- =============================================================================
\set ON_ERROR_STOP on

-- 0. ROLE
CREATE ROLE payroll_owner NOLOGIN NOINHERIT;
CREATE ROLE payroll_app      LOGIN NOINHERIT PASSWORD :'pw_app' CONNECTION LIMIT 40;
CREATE ROLE payroll_etl      LOGIN NOINHERIT PASSWORD :'pw_etl' CONNECTION LIMIT 5;
CREATE ROLE payroll_ro       LOGIN NOINHERIT PASSWORD :'pw_ro'  CONNECTION LIMIT 5;
CREATE ROLE payroll_migrator LOGIN NOINHERIT PASSWORD :'pw_mig' CONNECTION LIMIT 2;
GRANT payroll_owner TO payroll_migrator;

ALTER ROLE payroll_app      SET search_path = payroll, pg_catalog;
ALTER ROLE payroll_etl      SET search_path = payroll_stg, pg_catalog;
ALTER ROLE payroll_ro       SET search_path = payroll, pg_catalog;
ALTER ROLE payroll_migrator SET search_path = payroll, pg_catalog;
ALTER ROLE payroll_app      SET statement_timeout = '5min';
ALTER ROLE payroll_ro       SET default_transaction_read_only = on;
ALTER ROLE payroll_app      SET idle_in_transaction_session_timeout = '2min';

-- 1. DATABASE
CREATE DATABASE payroll OWNER payroll_owner ENCODING 'UTF8' TEMPLATE template0
       LC_COLLATE 'C' LC_CTYPE 'C';
REVOKE ALL     ON DATABASE payroll FROM PUBLIC;
GRANT  CONNECT ON DATABASE payroll TO payroll_app, payroll_etl, payroll_ro, payroll_migrator;
GRANT  TEMP    ON DATABASE payroll TO payroll_app;

-- Role payroll tidak boleh connect ke DB lain (pengganti pg_hba §13 di dev).
-- Eksplisit karena PUBLIC punya CONNECT secara default.
REVOKE CONNECT ON DATABASE postgres  FROM PUBLIC;
REVOKE CONNECT ON DATABASE template1 FROM PUBLIC;

ALTER ROLE payroll_migrator IN DATABASE payroll SET role = 'payroll_owner';

\connect payroll

-- 2. SCHEMA & HARDENING
REVOKE ALL ON SCHEMA public FROM PUBLIC;
DROP SCHEMA IF EXISTS public CASCADE;

CREATE SCHEMA payroll     AUTHORIZATION payroll_owner;
CREATE SCHEMA payroll_stg AUTHORIZATION payroll_owner;
REVOKE ALL ON SCHEMA payroll, payroll_stg FROM PUBLIC;

GRANT USAGE ON SCHEMA payroll     TO payroll_app, payroll_ro;
GRANT USAGE ON SCHEMA payroll_stg TO payroll_etl, payroll_app, payroll_ro;

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA payroll;

CREATE OR REPLACE FUNCTION payroll.trg_block_ext() RETURNS event_trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE r record;
BEGIN
  FOR r IN SELECT * FROM pg_event_trigger_ddl_commands() LOOP
    IF r.command_tag IN ('CREATE EXTENSION','CREATE FOREIGN DATA WRAPPER','CREATE SERVER') THEN
      IF r.object_identity ~* '(dblink|postgres_fdw|oracle_fdw|file_fdw)' OR r.command_tag <> 'CREATE EXTENSION' THEN
        RAISE EXCEPTION 'payroll DB: % % dilarang (isolasi)', r.command_tag, r.object_identity;
      END IF;
    END IF;
  END LOOP;
END $$;
CREATE EVENT TRIGGER block_cross_db ON ddl_command_end
  WHEN TAG IN ('CREATE EXTENSION','CREATE FOREIGN DATA WRAPPER','CREATE SERVER')
  EXECUTE FUNCTION payroll.trg_block_ext();

ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner REVOKE ALL ON TABLES    FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner REVOKE ALL ON SEQUENCES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner IN SCHEMA payroll
      GRANT SELECT ON TABLES TO payroll_ro;
ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner IN SCHEMA payroll_stg
      GRANT SELECT ON TABLES TO payroll_app, payroll_ro;
ALTER DEFAULT PRIVILEGES FOR ROLE payroll_owner IN SCHEMA payroll_stg
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO payroll_etl;
