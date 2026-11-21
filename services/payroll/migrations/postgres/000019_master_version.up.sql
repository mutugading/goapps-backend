-- [T007] Kolom versi row untuk ETag/If-Match master (07 Konvensi umum; D-16).
-- Naik +1 di setiap UPDATE oleh aplikasi (WHERE version = If-Match); 0 baris = 412.
ALTER TABLE payroll.pyr_employee     ADD COLUMN pe_version  integer NOT NULL DEFAULT 1;
ALTER TABLE payroll.pyr_company      ADD COLUMN pco_version integer NOT NULL DEFAULT 1;
ALTER TABLE payroll.pyr_division_ref ADD COLUMN pdr_version integer NOT NULL DEFAULT 1;
ALTER TABLE payroll.pyr_currency     ADD COLUMN pcy_version integer NOT NULL DEFAULT 1;
