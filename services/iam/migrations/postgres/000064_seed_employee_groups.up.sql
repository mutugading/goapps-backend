-- Seed 12 employee groups from the legacy Excel export.
-- Excel is authoritative for names: ON CONFLICT updates existing rows' names
-- (this renames legacy test fixtures ASM and DYM to their real meanings).
INSERT INTO mst_employee_group (code, name, is_active, created_by) VALUES
    ('ASM', 'Assistant Manager', true, 'seed'),
    ('DRM', 'Dirumahkan', true, 'seed'),
    ('DRV', 'Driver', true, 'seed'),
    ('DYM', 'Deputy Manager', true, 'seed'),
    ('EXCT', 'Executive', true, 'seed'),
    ('MGR', 'Manager', true, 'seed'),
    ('OPGS', 'Operator General Shift', true, 'seed'),
    ('OPKGS', 'Operator Kontrak GS', true, 'seed'),
    ('OPKS', 'Operator Kontrak Shift I,II,III', true, 'seed'),
    ('OPKWTGS', 'Opeartor PKWT GS Less 3 Months', true, 'seed'),
    ('OPS', 'Operator Shift I,II,III', true, 'seed'),
    ('SOASM', 'Solo Asmen', true, 'seed')
-- Partial unique index idx_mst_employee_group_code requires the matching WHERE predicate.
ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE SET name = EXCLUDED.name, updated_at = NOW(), updated_by = 'seed';
