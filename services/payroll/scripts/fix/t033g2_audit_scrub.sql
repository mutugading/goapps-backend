-- [T033g2] D-77 (2b): bersihkan baris pyr_audit_log yang ditulis SEBELUM allowlist audit (T033g1). Dulu MaskJSON memakai
-- denylist, jadi before/after lama bisa memuat Email, BirthDate, BirthPlace, Religion, Sex, BankAcntName, dst. Skrip ini
-- menerapkan allowlist yang SAMA dengan internal/infrastructure/audit/mask.go (allowedKeys; tes TestScrubScriptAllowlist
-- menjaga keduanya sejalan): key di luar allowlist → "***" di kedalaman mana pun.
--
-- Jalankan SEKALI per DB (dev, staging, production) sebagai owner, sesudah migrate + grants:
--   psql "postgres://payroll_migrator:...@<host>:5432/payroll_db?sslmode=require" -v ON_ERROR_STOP=1 -f scripts/fix/t033g2_audit_scrub.sql
-- Satu transaksi, idempoten (run kedua = 0 baris berubah). Tidak menyentuh baris ETL (aksi ETL_*: payload dibangun
-- jsonb_build_object dengan key tetap, tanpa PII). Keluaran: jumlah baris berubah per company, tanpa isi.
-- Audit: satu baris AUDIT_SCRUB per company yang berubah (actor 'owner-script', jumlah baris).

\set ON_ERROR_STOP on
BEGIN;

DO $$
BEGIN
  IF current_user <> 'payroll_owner' THEN
    RAISE EXCEPTION 'jalankan sebagai payroll_migrator (role payroll_owner), current_user=%', current_user;
  END IF;
END $$;

CREATE FUNCTION pg_temp.audit_mask(j jsonb, allow text[]) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
  CASE jsonb_typeof(j)
    WHEN 'object' THEN
      RETURN coalesce((SELECT jsonb_object_agg(k, CASE WHEN lower(k) = ANY (allow) THEN pg_temp.audit_mask(v, allow)
                                                      ELSE to_jsonb('***'::text) END)
                       FROM jsonb_each(j) e(k, v)), '{}'::jsonb);
    WHEN 'array' THEN
      RETURN coalesce((SELECT jsonb_agg(pg_temp.audit_mask(v, allow) ORDER BY o)
                       FROM jsonb_array_elements(j) WITH ORDINALITY a(v, o)), '[]'::jsonb);
    ELSE
      RETURN j;
  END CASE;
END $$;

CREATE TEMP TABLE audit_scrub_allow ON COMMIT DROP AS
SELECT ARRAY[
  -- ALLOWLIST BEGIN (salinan allowedKeys di mask.go, lower-case)
  'id', 'sysid', 'code', 'name', 'version', 'status', 'companyid', 'division', 'level',
  'createdby', 'createdat', 'updatedby', 'updatedat', 'action', 'before', 'after',
  'minorunit', 'takehomeround', 'basecurrency', 'active',
  'nik', 'nama', 'deptid', 'dept', 'sectionid', 'section', 'designationid', 'designation', 'grade',
  'taxid', 'stsemp', 'stspayroll', 'stsbonus', 'datejoin', 'resigndate', 'marriedsts', 'bank', 'bankbranch',
  'orionsubacnt', 'idmesin', 'cutitahunan', 'paycurrency', 'employmentstatus', 'costgroup', 'prevemployeeid',
  'hrissysid', 'placementreason',
  'type', 'scopedivisions', 'currency', 'defaultsts', 'taxclass', 'subsidyeligible', 'fixincome',
  'includeinestimate', 'tags', 'serial', 'basesysid', 'validfrom', 'validto', 'sts', 'tax', 'amount',
  'taxpaidcompany', 'frz', 'source', 'currentlevel', 'newlevel', 'changes', 'skippedmanual',
  'active_run_id', 'reason', 'run_id', 'superseded_run_id', 'yymm', 'rule_set_id', 'snapshot_id', 'input_hash',
  'emp_count', 'line_count', 'tot_tunjangan', 'tot_potongan', 'warnings',
  'jenis', 'divisi', 'baris', 'allow_open', 'pra_cek', 'pra_cek_transfer', 'tanpa_password', 'dilewati',
  'filter_nik', 'period_status', 'transfer_date',
  'total', 'queued', 'tanpa_email', 'email_ganda', 'sent', 'failed', 'blocked', 'pending', 'jumlah', 'tujuan',
  'item', 'asal', 'alasan', 'terkirim', 'tertunda', 'run', 'run_aktif'
  -- ALLOWLIST END
]::text[] AS allow;

CREATE TEMP TABLE audit_scrub_done ON COMMIT DROP AS
WITH upd AS (
  UPDATE payroll.pyr_audit_log l
  SET pal_before = pg_temp.audit_mask(l.pal_before, a.allow),
      pal_after  = pg_temp.audit_mask(l.pal_after, a.allow)
  FROM audit_scrub_allow a
  WHERE l.pal_action NOT LIKE 'ETL\_%'
    AND (l.pal_before IS DISTINCT FROM pg_temp.audit_mask(l.pal_before, a.allow)
      OR l.pal_after  IS DISTINCT FROM pg_temp.audit_mask(l.pal_after, a.allow))
  RETURNING l.pal_company_id
)
SELECT pal_company_id, count(*) AS n FROM upd GROUP BY 1;

INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_company_id, pal_after)
SELECT 'owner-script', 'AUDIT_SCRUB', 'pyr_audit_log', pal_company_id, jsonb_build_object('jumlah', n, 'reason', 'T033g2 D-77 (2b)')
FROM audit_scrub_done;

SELECT 'company ' || pal_company_id || ': ' || n || ' baris audit dimask' FROM audit_scrub_done
UNION ALL SELECT 'total: ' || coalesce(sum(n), 0) || ' baris' FROM audit_scrub_done;

COMMIT;
