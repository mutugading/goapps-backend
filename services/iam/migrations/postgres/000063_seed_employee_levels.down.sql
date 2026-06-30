-- Revert seeded employee levels and drop the legacy workflow_code column.
DELETE FROM mst_employee_level WHERE created_by = 'seed' AND code IN (
    'APRV', 'E-16', 'E-17', 'E-18', 'E-19', 'EG-6', 'G-4', 'G-5', 'G-6', 'HR-CFR', 'HS-23', 'HS-24', 'HS-25', 'I-13', 'I-14', 'I-15', 'M-10', 'M-11', 'M-12', 'P-7', 'P-8', 'P-9', 'S-26', 'S-27', 'S-28', 'SS-20', 'SS-21', 'SS-22', 'SU'
);
ALTER TABLE mst_employee_level DROP COLUMN IF EXISTS workflow_code;
