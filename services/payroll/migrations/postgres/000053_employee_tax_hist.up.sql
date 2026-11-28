-- [T033n] Riwayat status pajak (D-80 (3)). Legacy tidak menyimpan riwayat; PTKP tahunan (Desember dan true-up
-- leaver, pajak v2 D-81) memakai status per 1 Januari tahun pajak, bukan status terkini.
-- * Satu baris per (karyawan, tanggal berlaku). Ditulis HANYA oleh trigger di pyr_employee (master service
--   maupun ETL), jadi app tidak punya INSERT/UPDATE/DELETE (deploy/grants.sql bagian 3).
-- * INSERT karyawan: berlaku mulai 1 Januari tahun berjalan. UPDATE pe_tax_id: berlaku mulai hari itu
--   (perubahan kedua di hari yang sama menimpa baris hari itu, sumber jadi UPDATE; jejak nilai lama tetap di
--   audit master). pe_tax_id dikosongkan tidak dicatat (engine lalu memakai status terkini kosong = PPH_NO_PTKP).
-- * Tanggal = hari kalender WIB (Asia/Jakarta), tidak bergantung TimeZone sesi.
-- * Koreksi salah input yang harus berlaku mundur ke 1 Januari belum punya jalur (D-82 PROPOSED, T033n2).
-- * Backfill: status terkini berlaku mulai 1 Januari tahun pajak berjalan.
-- * Pembaca (loader): baris terakhir dengan berlaku <= 1 Jan; kalau tidak ada (karyawan baru / status baru
--   diisi tengah tahun), baris paling awal.
CREATE TABLE IF NOT EXISTS payroll.pyr_employee_tax_hist (
  peth_employee_id bigint      NOT NULL REFERENCES payroll.pyr_employee (pe_id) ON DELETE CASCADE,
  peth_valid_from  date        NOT NULL,
  peth_tax_id      varchar(10) NOT NULL,
  peth_source      varchar(10) NOT NULL CHECK (peth_source IN ('BACKFILL','INSERT','UPDATE')),
  created_by       varchar(64) NOT NULL DEFAULT session_user,
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (peth_employee_id, peth_valid_from)
);

CREATE OR REPLACE FUNCTION payroll.fn_employee_tax_hist() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
BEGIN
  IF NEW.pe_tax_id IS NULL OR (TG_OP = 'UPDATE' AND NEW.pe_tax_id IS NOT DISTINCT FROM OLD.pe_tax_id) THEN
    RETURN NULL;
  END IF;
  INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source, created_by)
  VALUES (NEW.pe_id,
          CASE TG_OP WHEN 'INSERT' THEN date_trunc('year', (now() AT TIME ZONE 'Asia/Jakarta'))::date
                      ELSE (now() AT TIME ZONE 'Asia/Jakarta')::date END,
          NEW.pe_tax_id, TG_OP, COALESCE(NEW.updated_by, NEW.created_by, session_user))
  ON CONFLICT (peth_employee_id, peth_valid_from)
  DO UPDATE SET peth_tax_id = EXCLUDED.peth_tax_id, peth_source = EXCLUDED.peth_source,
                created_by = EXCLUDED.created_by, created_at = now();
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS trg_employee_tax_hist ON payroll.pyr_employee;
CREATE TRIGGER trg_employee_tax_hist AFTER INSERT ON payroll.pyr_employee
  FOR EACH ROW EXECUTE FUNCTION payroll.fn_employee_tax_hist();
DROP TRIGGER IF EXISTS trg_employee_tax_hist_upd ON payroll.pyr_employee;
CREATE TRIGGER trg_employee_tax_hist_upd AFTER UPDATE OF pe_tax_id ON payroll.pyr_employee
  FOR EACH ROW WHEN (OLD.pe_tax_id IS DISTINCT FROM NEW.pe_tax_id) -- upsert ETL tanpa perubahan tidak memanggil fungsi
  EXECUTE FUNCTION payroll.fn_employee_tax_hist();

INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source)
SELECT pe_id, date_trunc('year', (now() AT TIME ZONE 'Asia/Jakarta'))::date, pe_tax_id, 'BACKFILL'
FROM payroll.pyr_employee WHERE pe_tax_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- RLS (I20) mengikuti induk; bentuk sesudah 000049: p_parent khusus app, ro baca lewat p_ro.
ALTER TABLE payroll.pyr_employee_tax_hist ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_parent ON payroll.pyr_employee_tax_hist;
CREATE POLICY p_parent ON payroll.pyr_employee_tax_hist TO payroll_app
  USING (EXISTS (SELECT 1 FROM payroll.pyr_employee e WHERE e.pe_id = peth_employee_id))
  WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_employee e WHERE e.pe_id = peth_employee_id));
DROP POLICY IF EXISTS p_ro ON payroll.pyr_employee_tax_hist;
CREATE POLICY p_ro ON payroll.pyr_employee_tax_hist FOR SELECT TO payroll_ro USING (true);
