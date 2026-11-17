-- [T028g4] D-69 (1)(3): seed akun email + Reply-To HR per divisi di payroll.pyr_mail_account (SOLO, JAKARTA).
-- Idempoten: upsert lewat unique parsial aktif (company, COALESCE(divisi,'')) (migrasi 000044). Dijalankan
-- sebagai role owner/migrator (payroll_app hanya SELECT pada tabel ini, grants.sql / D-62). Tanpa nilai nyata di repo:
-- semua nilai masuk lewat variabel psql.
--
--   psql "postgres://payroll_migrator@<HOST>:5432/payroll_db?sslmode=require" \
--     -v company_id=<CID> -v host=<SMTP_HOST> -v from_address=<FROM> -v secret_ref=<NAMA_SECRET> \
--     -v reply_to_solo=<HR_SOLO> -v reply_to_jakarta=<HR_JAKARTA> \
--     [-v port=587] [-v from_name='HR MGT'] -f services/payroll/deploy/seed_mail_account.sql
--
-- Wajib: company_id, host, from_address, secret_ref, reply_to_solo, reply_to_jakarta (kosong/tidak diisi =
-- skrip berhenti dengan exit non-nol, tidak ada perubahan). Opsional: port (587), from_name (NULL).
-- pma_secret_ref hanya NAMA secret; kredensial SMTP tidak disimpan di DB.
-- Tanpa seed ini Reply-To jatuh ke env PAYROLL_SMTP_REPLY_TO (D-69 (3)).
\set ON_ERROR_STOP on
\set QUIET on

-- Variabel yang tidak diberikan dianggap kosong (referensi variabel tak terdefinisi akan jadi SQL rusak).
\if :{?company_id}      \else \set company_id ''      \endif
\if :{?host}            \else \set host ''            \endif
\if :{?from_address}    \else \set from_address ''    \endif
\if :{?secret_ref}      \else \set secret_ref ''      \endif
\if :{?reply_to_solo}   \else \set reply_to_solo ''   \endif
\if :{?reply_to_jakarta} \else \set reply_to_jakarta '' \endif
\if :{?port}            \else \set port '587'         \endif
\if :{?from_name}       \else \set from_name ''       \endif

SELECT (btrim(:'company_id') = '' OR btrim(:'host') = '' OR btrim(:'from_address') = ''
        OR btrim(:'secret_ref') = '' OR btrim(:'reply_to_solo') = '' OR btrim(:'reply_to_jakarta') = ''
        OR btrim(:'port') = '') AS missing \gset

\if :missing
  \echo 'seed_mail_account: variabel wajib kosong (company_id, host, from_address, secret_ref, reply_to_solo, reply_to_jakarta); tidak ada perubahan.'
  DO $$ BEGIN RAISE EXCEPTION 'seed_mail_account: variabel wajib kosong'; END $$;
\endif

BEGIN;

INSERT INTO payroll.pyr_mail_account
  (pma_company_id, pma_division, pma_host, pma_port, pma_from_address, pma_from_name, pma_reply_to, pma_secret_ref, pma_active)
SELECT :'company_id'::smallint, v.div::payroll.division, :'host', :'port'::int, :'from_address',
       NULLIF(:'from_name', ''), v.reply_to, :'secret_ref', true
FROM (VALUES ('SOLO', :'reply_to_solo'), ('JAKARTA', :'reply_to_jakarta')) AS v(div, reply_to)
ON CONFLICT (pma_company_id, COALESCE(pma_division::text, '')) WHERE pma_active
DO UPDATE SET pma_host = EXCLUDED.pma_host, pma_port = EXCLUDED.pma_port,
              pma_from_address = EXCLUDED.pma_from_address, pma_from_name = EXCLUDED.pma_from_name,
              pma_reply_to = EXCLUDED.pma_reply_to, pma_secret_ref = EXCLUDED.pma_secret_ref;

COMMIT;

\unset QUIET
\echo 'seed_mail_account: akun aktif company' :company_id
SELECT pma_division, pma_host, pma_port, pma_from_address, pma_reply_to, pma_secret_ref
FROM payroll.pyr_mail_account
WHERE pma_company_id = :'company_id'::smallint AND pma_active
ORDER BY pma_division NULLS FIRST;
