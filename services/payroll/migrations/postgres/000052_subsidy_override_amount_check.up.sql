-- [T033i] Override subsidi: nominal tidak negatif, dan AMOUNT wajib bernominal (review #283 PPh minor).
-- Engine juga membatasi P0 ke P1. Gagal dengan pesan jelas bila ada baris lama yang melanggar.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM payroll.pyr_subsidy_override
             WHERE pso_amount < 0 OR (pso_action = 'AMOUNT' AND pso_amount IS NULL)) THEN
    RAISE EXCEPTION 'pyr_subsidy_override: ada pso_amount negatif atau AMOUNT tanpa nominal; perbaiki dulu';
  END IF;
END $$;
ALTER TABLE payroll.pyr_subsidy_override DROP CONSTRAINT IF EXISTS chk_pso_amount_nonneg;
ALTER TABLE payroll.pyr_subsidy_override ADD CONSTRAINT chk_pso_amount_nonneg
  CHECK (COALESCE(pso_amount >= 0, pso_action <> 'AMOUNT')); -- NULL hanya untuk INCLUDE/EXCLUDE
