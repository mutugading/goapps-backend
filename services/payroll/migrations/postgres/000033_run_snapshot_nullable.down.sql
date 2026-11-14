ALTER TABLE payroll.pyr_run DROP CONSTRAINT IF EXISTS ck_pr_snapshot_engine;
-- Down gagal bila sudah ada run LEGACY ber-snapshot NULL (201712-202003); hapus run itu dulu.
ALTER TABLE payroll.pyr_run ALTER COLUMN pr_snapshot_id SET NOT NULL;
