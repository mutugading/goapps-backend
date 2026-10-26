-- [T009e5] D-35.3: periode 201712-202003 tidak punya rekap absensi, jadi run LEGACY-nya tanpa rujukan snapshot.
-- Run ENGINE tetap wajib bersnapshot (CHECK); hanya run LEGACY yang boleh NULL.
ALTER TABLE payroll.pyr_run ALTER COLUMN pr_snapshot_id DROP NOT NULL;
ALTER TABLE payroll.pyr_run ADD CONSTRAINT ck_pr_snapshot_engine CHECK (pr_kind = 'LEGACY' OR pr_snapshot_id IS NOT NULL);
