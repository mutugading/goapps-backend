-- Add workflow_code column to preserve the legacy numeric Workflow Code, then seed
-- 29 employee levels from the legacy Excel export.
-- The existing `workflow` column keeps its lifecycle meaning (RELEASED for seeded data).
ALTER TABLE mst_employee_level ADD COLUMN IF NOT EXISTS workflow_code SMALLINT;
COMMENT ON COLUMN mst_employee_level.workflow_code IS 'Legacy numeric workflow/approval code imported from the old HR system (nullable).';

-- type: 1=EXECUTIVE, 3=OPERATOR | workflow lifecycle: 2=RELEASED
INSERT INTO mst_employee_level (code, name, grade, type, sequence, workflow, workflow_code, is_active, created_by) VALUES
    ('APRV', 'HR Approval', 0, 1, 26, 2, 1, true, 'seed'),
    ('E-16', 'Senior Executive', 12, 1, 13, 2, 2, true, 'seed'),
    ('E-17', 'Executive', 11, 1, 14, 2, 1, true, 'seed'),
    ('E-18', 'Assistant Executive', 10, 1, 15, 2, 1, true, 'seed'),
    ('E-19', 'Junior Executive', 9, 1, 16, 2, 1, true, 'seed'),
    ('EG-6', 'Expat General Manager', 23, 1, 27, 2, 3, true, 'seed'),
    ('G-4', 'Senior Vice President', 25, 1, 1, 2, 3, true, 'seed'),
    ('G-5', 'Vice President', 24, 1, 2, 2, 2, true, 'seed'),
    ('G-6', 'General Manager', 22, 1, 3, 2, 3, true, 'seed'),
    ('HR-CFR', 'HR Confirmed', 0, 1, 28, 2, 4, true, 'seed'),
    ('HS-23', 'Highly Skilled Operator', 5, 3, 20, 2, 0, true, 'seed'),
    ('HS-24', 'Highly Skilled Operator', 4, 3, 21, 2, 0, true, 'seed'),
    ('HS-25', 'Highly Skilled Operator', 3, 3, 22, 2, 0, true, 'seed'),
    ('I-13', 'Joint Manager', 15, 1, 10, 2, 3, true, 'seed'),
    ('I-14', 'Deputy Manager', 14, 1, 11, 2, 2, true, 'seed'),
    ('I-15', 'Assistant Manager', 13, 1, 12, 2, 2, true, 'seed'),
    ('M-10', 'Chift Manager', 18, 1, 7, 2, 3, true, 'seed'),
    ('M-11', 'Senior Manager', 17, 1, 8, 2, 3, true, 'seed'),
    ('M-12', 'Manager', 16, 1, 9, 2, 3, true, 'seed'),
    ('P-7', 'Joint General Manager', 21, 1, 4, 2, 3, true, 'seed'),
    ('P-8', 'Deputy General Manager', 20, 1, 5, 2, 3, true, 'seed'),
    ('P-9', 'Assistant General Manager', 19, 1, 6, 2, 3, true, 'seed'),
    ('S-26', 'Skilled Operator', 2, 3, 23, 2, 0, true, 'seed'),
    ('S-27', 'Skilled Operator', 1, 3, 24, 2, 0, true, 'seed'),
    ('S-28', 'Skilled Operator', 0, 3, 25, 2, 0, true, 'seed'),
    ('SS-20', 'Super Skilled Operator', 8, 3, 17, 2, 0, true, 'seed'),
    ('SS-21', 'Super Skilled Operator', 7, 3, 18, 2, 0, true, 'seed'),
    ('SS-22', 'Super Skilled Operator', 6, 3, 19, 2, 0, true, 'seed'),
    ('SU', 'Super User', 0, 1, 99, 2, 99, true, 'seed')
-- Existing rows (e.g. SU) keep their data; only backfill the new legacy workflow_code.
-- Partial unique index idx_mst_employee_level_code requires the matching WHERE predicate.
ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE SET workflow_code = EXCLUDED.workflow_code;
