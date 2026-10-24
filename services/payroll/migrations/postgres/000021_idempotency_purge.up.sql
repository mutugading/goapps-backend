-- [T007f] Purge massal pyr_idempotency_key kedaluwarsa (D-19).
-- RLS p_self hanya menampilkan baris user sendiri, jadi purge lintas user lewat fungsi
-- SECURITY DEFINER (pemilik tabel = migrator, tanpa FORCE RLS). Hanya menghapus baris yang
-- sudah kedaluwarsa lebih dari p_grace; tidak menerima input lain.
CREATE OR REPLACE FUNCTION payroll.fn_purge_idempotency(p_grace interval DEFAULT interval '1 day') RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_n bigint;
BEGIN
  DELETE FROM payroll.pyr_idempotency_key WHERE pik_expires_at < now() - p_grace;
  GET DIAGNOSTICS v_n = ROW_COUNT;
  RETURN v_n;
END $$;
REVOKE ALL ON FUNCTION payroll.fn_purge_idempotency(interval) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION payroll.fn_purge_idempotency(interval) TO payroll_app;
