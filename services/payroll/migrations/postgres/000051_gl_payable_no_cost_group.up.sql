-- [T033h3] Baris PAYABLE GL tidak lagi dikunci per cost group (SALARY001 join Salary Payables hanya jabatan + divisi).
-- Ekspor GL kini hanya membaca PAYABLE tanpa cost group; baris PAYABLE hasil ETL lama yang masih ber-cost group
-- dihapus supaya tidak ada dua kandidat akun per grup. Baris buatan app (pgs_legacy_row NULL) tidak disentuh.
-- Sesudah migrasi ini jalankan ulang `payroll-sync gl-setup` (runbook) agar baris PAYABLE tanpa cost group terisi.
DELETE FROM payroll.pyr_gl_setup
WHERE pgs_line_kind = 'PAYABLE' AND pgs_cost_group IS NOT NULL AND pgs_legacy_row IS NOT NULL;
