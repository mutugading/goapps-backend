-- [T009d] ETL rekap absensi historis → snapshot LEGACY (08§2, 05§6). payroll_etl menulis pyr_att_snapshot(_row).
-- Snapshot hasil ETL ditandai pas_source_batch_ids = '{}' (tanpa batch staging) dan unik per (company, yymm, division),
-- sehingga run ulang meng-upsert, bukan menambah snapshot.
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_att_snapshot, payroll.pyr_att_snapshot_row TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_etl;

CREATE UNIQUE INDEX uix_pas_legacy ON payroll.pyr_att_snapshot (pas_company_id, pas_yymm, pas_division)
  WHERE pas_source_batch_ids = '{}';

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_att_snapshot','pyr_att_snapshot_row'] LOOP
    IF NOT (SELECT relrowsecurity FROM pg_class WHERE oid = ('payroll.' || t)::regclass) THEN
      RAISE EXCEPTION 'payroll.% tidak ber-RLS; p_etl tidak bisa dipasang', t;
    END IF;
    EXECUTE format('DROP POLICY IF EXISTS p_etl ON payroll.%I', t);
    EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
  END LOOP;
END $$;

COMMENT ON COLUMN payroll.pyr_att_snapshot_row.pasr_kind IS 'REKAP | ADD | SCH (SCH: satu baris per NIK, pasr_data = {"SCHEDULES": {kode: jumlah}}; hanya snapshot LEGACY)';
