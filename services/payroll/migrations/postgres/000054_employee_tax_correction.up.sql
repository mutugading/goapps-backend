-- [T033n2] Koreksi status pajak bertanggal mundur (D-82 (5) opsi (a), rincian D-82 (6)).
-- Salah input pe_tax_id (bukan perubahan status sungguhan) dicatat trigger 000053 berlaku hari koreksi, sehingga PTKP
-- tahunan tetap memakai status salah. Jalur koreksi: "status pada tanggal D adalah X", D di tahun pajak berjalan atau
-- tahun lalu, D <= hari ini (WIB). X berlaku sejak D sampai perubahan tercatat berikutnya.
-- * Riwayat: baris (D, X, 'CORRECT', alasan) di-upsert. Baris lain tidak dihapus: perubahan sungguhan sesudah D
--   (mis. menikah Maret) tetap berlaku sejak tanggalnya.
-- * pe_tax_id (status terkini, dasar TER bulanan) hanya ikut menjadi X bila tidak ada baris riwayat sesudah D; kalau
--   ada, status terkini milik perubahan sesudahnya dan tidak disentuh (versi tetap naik untuk If-Match).
-- * Tabel riwayat tetap read-only untuk app; satu-satunya jalur tulis app = fungsi SECURITY DEFINER ini
--   (EXECUTE hanya payroll_app, deploy/grants.sql). Aktor dari app.user_id sesi (ScopedTx), bukan parameter.
--   Permission, If-Match, dan audit (TAX_STATUS_CORRECT) dikerjakan service dalam transaksi yang sama.
-- * Lapis DB: karyawan dalam app.scopes; kode X = EMPTAX rule set ACTIVE; untuk SETIAP tahun pajak dari tahun D
--   sampai tahun berjalan, periode Desember atau periode yang memuat tanggal resign di tahun itu tidak boleh
--   APPROVED/CLOSED (P0001). Baris periode itu dikunci FOR SHARE sehingga approve/close yang berjalan bersamaan
--   menunggu transaksi koreksi selesai.
ALTER TABLE payroll.pyr_employee_tax_hist DROP CONSTRAINT IF EXISTS pyr_employee_tax_hist_peth_source_check;
ALTER TABLE payroll.pyr_employee_tax_hist ADD CONSTRAINT pyr_employee_tax_hist_peth_source_check
  CHECK (peth_source IN ('BACKFILL','INSERT','UPDATE','CORRECT'));
ALTER TABLE payroll.pyr_employee_tax_hist ADD COLUMN IF NOT EXISTS peth_reason varchar(500);
ALTER TABLE payroll.pyr_employee_tax_hist DROP CONSTRAINT IF EXISTS chk_peth_reason;
ALTER TABLE payroll.pyr_employee_tax_hist ADD CONSTRAINT chk_peth_reason
  CHECK (peth_source <> 'CORRECT' OR length(btrim(COALESCE(peth_reason, ''))) > 0);

CREATE OR REPLACE FUNCTION payroll.fn_correct_employee_tax(p_employee_id bigint, p_tax_id varchar, p_valid_from date,
  p_reason varchar) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE
  v_today date := (now() AT TIME ZONE 'Asia/Jakarta')::date;
  v_actor text := NULLIF(btrim(current_setting('app.user_id', true)), '');
  v_co smallint; v_div text; v_resign date; v_version int; v_bad record; v_later boolean;
BEGIN
  IF p_tax_id IS NULL OR btrim(p_tax_id) = '' OR p_valid_from IS NULL OR p_reason IS NULL OR btrim(p_reason) = ''
     OR v_actor IS NULL THEN
    RAISE EXCEPTION 'koreksi status pajak: tax_id, valid_from, alasan, dan app.user_id wajib' USING ERRCODE = '22023';
  END IF;
  IF p_valid_from > v_today OR extract(year FROM p_valid_from)::int < extract(year FROM v_today)::int - 1 THEN
    RAISE EXCEPTION 'koreksi status pajak: valid_from % di luar tahun pajak berjalan/lalu atau di masa depan', p_valid_from
      USING ERRCODE = '22023';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM payroll.pyr_static_value v JOIN payroll.pyr_rule_set r ON r.prs_id = v.psv_rule_set_id
                 WHERE r.prs_status = 'ACTIVE' AND v.psv_vs_code = 'EMPTAX' AND v.psv_code = p_tax_id) THEN
    RAISE EXCEPTION 'koreksi status pajak: % bukan kode EMPTAX rule set aktif', p_tax_id USING ERRCODE = '22023';
  END IF;
  SELECT pe_company_id, pe_division, pe_resign_date INTO v_co, v_div, v_resign
  FROM payroll.pyr_employee WHERE pe_id = p_employee_id FOR UPDATE;
  IF NOT FOUND OR NOT payroll.fn_in_scope(v_co, v_div) THEN
    RAISE EXCEPTION 'koreksi status pajak: karyawan % tidak ditemukan', p_employee_id USING ERRCODE = 'P0002';
  END IF;
  -- Periode final tiap tahun pajak yang terdampak (tahun D s.d. tahun berjalan). Kunci dulu, lalu periksa status.
  FOR v_bad IN
    SELECT pp_yymm, pp_status FROM payroll.pyr_period p
    WHERE pp_company_id = v_co AND pp_division = v_div
      AND pp_yymm / 100 BETWEEN extract(year FROM p_valid_from)::int AND extract(year FROM v_today)::int
      AND (pp_yymm % 100 = 12 OR (v_resign IS NOT NULL AND v_resign BETWEEN pp_date_from AND pp_date_to))
    ORDER BY pp_yymm FOR SHARE
  LOOP
    IF v_bad.pp_status IN ('APPROVED', 'CLOSED') THEN
      RAISE EXCEPTION 'Periode % % % sudah %, status pajak tidak boleh dikoreksi', v_co, v_bad.pp_yymm, v_div, v_bad.pp_status
        USING ERRCODE = 'P0001';
    END IF;
  END LOOP;

  v_later := EXISTS (SELECT 1 FROM payroll.pyr_employee_tax_hist
                     WHERE peth_employee_id = p_employee_id AND peth_valid_from > p_valid_from);
  -- Tanpa perubahan sesudah D, status terkini = X (trigger 000053 menulis baris hari ini bila berubah; konsisten).
  UPDATE payroll.pyr_employee
  SET pe_tax_id = CASE WHEN v_later THEN pe_tax_id ELSE p_tax_id END,
      updated_by = v_actor, updated_at = now(), pe_version = pe_version + 1
  WHERE pe_id = p_employee_id RETURNING pe_version INTO v_version;
  IF NOT v_later THEN -- baris hari ini dari trigger 000053 hanya gema koreksi ini (X sudah berlaku sejak D)
    DELETE FROM payroll.pyr_employee_tax_hist
    WHERE peth_employee_id = p_employee_id AND peth_valid_from > p_valid_from;
  END IF;
  INSERT INTO payroll.pyr_employee_tax_hist (peth_employee_id, peth_valid_from, peth_tax_id, peth_source, peth_reason, created_by)
  VALUES (p_employee_id, p_valid_from, p_tax_id, 'CORRECT', btrim(p_reason), v_actor)
  ON CONFLICT (peth_employee_id, peth_valid_from)
  DO UPDATE SET peth_tax_id = EXCLUDED.peth_tax_id, peth_source = 'CORRECT', peth_reason = EXCLUDED.peth_reason,
                created_by = EXCLUDED.created_by, created_at = now();
  RETURN v_version;
END $$;
