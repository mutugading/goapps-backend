-- Revert seeded employee groups (only rows created by this seed; renamed fixtures are not restored).
DELETE FROM mst_employee_group WHERE created_by = 'seed' AND code IN (
    'ASM', 'DRM', 'DRV', 'DYM', 'EXCT', 'MGR', 'OPGS', 'OPKGS', 'OPKS', 'OPKWTGS', 'OPS', 'SOASM'
);
