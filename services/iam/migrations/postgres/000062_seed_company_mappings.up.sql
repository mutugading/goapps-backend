-- Seed company-mapping master hierarchy (company > division > department > section)
-- and the 76 company mappings, imported from the legacy system Excel export.
-- Idempotent: reuses existing rows by code (ON CONFLICT DO NOTHING); FKs resolved by code.

-- 1. Company (reuse existing MGT if present)
INSERT INTO mst_company (company_code, company_name, is_active, created_by) VALUES
    ('MGT', 'Mutu Gading Tekstil', true, 'seed')
ON CONFLICT (company_code) DO NOTHING;

-- 2. Divisions
INSERT INTO mst_division (company_id, division_code, division_name, is_active, created_by) VALUES
    ((SELECT company_id FROM mst_company WHERE company_code = 'MGT'), 'JKT', 'Jakarta', true, 'seed'),
    ((SELECT company_id FROM mst_company WHERE company_code = 'MGT'), 'SLO', 'Solo', true, 'seed')
ON CONFLICT (division_code) DO NOTHING;

-- 3. Departments
INSERT INTO mst_department (division_id, department_code, department_name, is_active, created_by) VALUES
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'CIM', 'Continous Improvement', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'FIN', 'Finance', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'JKT'), 'GNRL', 'General', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'HRM', 'Human Resources Management', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'HSS', 'Health Safety Environment and Security', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'IT', 'Information Technology', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'ITHW', 'IT Hardware', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'ITS', 'IT Software', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'MB', 'Masterbatch', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'MC', 'Material Control', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'PPC', 'Planning and Production Control', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'RND', 'Research and Development', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'SPBA', 'Superba', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'SPG', 'Spinning', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TPMC', 'TPM Civil', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TPME', 'TPM Electric', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TPMG', 'TPM General', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TPMU', 'TPM Utility', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TQM', 'TQM Control', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TWT', 'Twisting', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'TXT', 'Texturising', true, 'seed'),
    ((SELECT division_id FROM mst_division WHERE division_code = 'SLO'), 'UNH', 'Unit Head', true, 'seed')
ON CONFLICT (department_code) DO NOTHING;

-- 4. Sections
INSERT INTO mst_section (department_id, section_code, section_name, is_active, created_by) VALUES
    ((SELECT department_id FROM mst_department WHERE department_code = 'CIM'), 'CIMGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINACNT', 'Accounting', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINCOST', 'Costing', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINMIS', 'MIS', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINPAY', 'Payables', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINREC', 'Receivables', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'FIN'), 'FINTAX', 'Tax', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'GNRL'), 'GNRLGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMATT', 'Attendance', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMDRV', 'Driver', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMLGL', 'Legal Permission', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMRECRUT', 'Recruitment', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMSCRTY', 'Security', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMTRAIN', 'Training', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMTRNSPT', 'Transportation', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HRM'), 'HRMWLFR', 'Welfare', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HSS'), 'HSSSCRTY', 'Security', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'HSS'), 'HSSSFTY', 'Safety', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'IT'), 'ITHARD', 'Hardware', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'ITHW'), 'ITHWHARD', 'Hardware', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'IT'), 'ITSOFT', 'Software', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'ITS'), 'ITSSOFT', 'Software', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MB'), 'MBGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCDES', 'Despatch', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCIMP', 'Import', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCPURCH', 'Purchase', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCSTRS', 'Store', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'MC'), 'MCWHS', 'Warehouse', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'PPC'), 'PPCEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'PPC'), 'PPCPPC', 'PPC', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'RND'), 'RNDEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'RND'), 'RNDRND', 'Research and Development', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPBA'), 'SPBAEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGBO', 'Burn Out', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGMECH', 'Mechanic', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGMELT', 'Melting', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGPACK', 'Packing', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGPC', 'Process Control', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGPOP', 'Popcorn', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGPPRTB', 'Paper Tube', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGPROD', 'Production', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'SPG'), 'SPGTAKE', 'Take Up', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPMC'), 'TPMCCVL', 'Civil', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPME'), 'TPMEELECT', 'Electric', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPME'), 'TPMEEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPMG'), 'TPMGGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPMU'), 'TPMUCVL', 'Civil', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPMU'), 'TPMUEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TPMU'), 'TPMUUTIL', 'Utility', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMLABCHEM', 'Laboratorium Chemical', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMLABTEKS', 'Laboratorium Tekstil', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMPC', 'Process Control', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TQM'), 'TQMQA', 'Quality Assurance', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTCOLKIT', 'Color Kitchen', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTMECH', 'Mechanic', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTPACK', 'Packing', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTPC', 'Process Control', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTPPC', 'PPC', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTPROD', 'Production', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TWT'), 'TWTRND', 'Research and Development', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTEXPT', 'Expatriat', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTGEN', 'General', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTMECH', 'Mechanic', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTPACK', 'Packing', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTPPC', 'PPC', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'TXT'), 'TXTPROD', 'Production', true, 'seed'),
    ((SELECT department_id FROM mst_department WHERE department_code = 'UNH'), 'UNHEXPT', 'Expatriat', true, 'seed')
ON CONFLICT (section_code) DO NOTHING;

-- 5. Company mappings
-- Each insert is guarded twice: ON CONFLICT (code) skips duplicate codes, and
-- WHERE NOT EXISTS skips rows whose (company,division,department,section) combo is
-- already taken by another mapping (unique_mapping_combo) -- e.g. a reused section.
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTJKTGNRLGEN', 'General - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'JKT') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'GNRL') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'GNRLGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOCIMGEN', 'Continous Improvement - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'CIM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'CIMGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINACNT', 'Finance - Accounting', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINACNT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINCOST', 'Finance - Costing', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINCOST') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINEXPT', 'Finance - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINMIS', 'Finance - MIS', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINMIS') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINPAY', 'Finance - Payables', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINPAY') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINREC', 'Finance - Receivables', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINREC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOFINTAX', 'Finance - Tax', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'FIN') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'FINTAX') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMATT', 'Human Resources Management - Attendance', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMATT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMDRV', 'Human Resources Management - Driver', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMDRV') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMGEN', 'Human Resources Management - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMLGL', 'Human Resources Management - Legal Permission', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMLGL') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMRECRUT', 'Human Resources Management - Recruitment', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMRECRUT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMSCRTY', 'Human Resources Management - Security', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMSCRTY') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMTRAIN', 'Human Resources Management - Training', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMTRAIN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMTRNSPT', 'Human Resources Management - Transportation', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMTRNSPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHRMWLFR', 'Human Resources Management - Welfare', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HRM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HRMWLFR') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHSSSCRTY', 'Health Safety Environment and Security - Security', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HSS') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HSSSCRTY') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOHSSSFTY', 'Health Safety Environment and Security - Safety', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'HSS') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'HSSSFTY') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOITHARD', 'Information Technology - Hardware', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'IT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'ITHARD') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOITHWHARD', 'IT Hardware - Hardware', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'ITHW') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'ITHWHARD') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOITSOFT', 'Information Technology - Software', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'IT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'ITSOFT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOITSSOFT', 'IT Software - Software', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'ITS') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'ITSSOFT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMBGEN', 'Masterbatch - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MB') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MBGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCDES', 'Material Control - Despatch', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCDES') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCEXPT', 'Material Control - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCIMP', 'Material Control - Import', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCIMP') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCPURCH', 'Material Control - Purchase', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCPURCH') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCSTRS', 'Material Control - Store', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCSTRS') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOMCWHS', 'Material Control - Warehouse', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'MC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'MCWHS') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOPPCEXPT', 'Planning and Production Control - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'PPC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'PPCEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOPPCPPC', 'Planning and Production Control - PPC', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'PPC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'PPCPPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLORNDEXPT', 'Research and Development - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'RND') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'RNDEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLORNDRND', 'Research and Development - Research and Development', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'RND') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'RNDRND') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPBAEXPT', 'Superba - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPBA') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPBAEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGBO', 'Spinning - Burn Out', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGBO') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGEXPT', 'Spinning - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGGEN', 'Spinning - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGMECH', 'Spinning - Mechanic', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGMECH') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGMELT', 'Spinning - Melting', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGMELT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGPACK', 'Spinning - Packing', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGPACK') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGPC', 'Spinning - Process Control', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGPOP', 'Spinning - Popcorn', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGPOP') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGPPRTB', 'Spinning - Paper Tube', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGPPRTB') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGPROD', 'Spinning - Production', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGPROD') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOSPGTAKE', 'Spinning - Take Up', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'SPG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'SPGTAKE') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMCCVL', 'TPM Civil - Civil', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPMC') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMCCVL') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMEELECT', 'TPM Electric - Electric', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPME') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMEELECT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMEEXPT', 'TPM Electric - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPME') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMEEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMGGEN', 'TPM General - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPMG') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMGGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMUCVL', 'TPM Utility - Civil', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPMU') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMUCVL') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMUEXPT', 'TPM Utility - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPMU') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMUEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTPMUUTIL', 'TPM Utility - Utility', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TPMU') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TPMUUTIL') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMEXPT', 'TQM Control - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMGEN', 'TQM Control - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMLABCHEM', 'TQM Control - Laboratorium Chemical', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMLABCHEM') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMLABTEKS', 'TQM Control - Laboratorium Tekstil', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMLABTEKS') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMPC', 'TQM Control - Process Control', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTQMQA', 'TQM Control - Quality Assurance', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TQM') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TQMQA') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTCOLKIT', 'Twisting - Color Kitchen', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTCOLKIT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTEXPT', 'Twisting - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTGEN', 'Twisting - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTMECH', 'Twisting - Mechanic', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTMECH') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTPACK', 'Twisting - Packing', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTPACK') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTPC', 'Twisting - Process Control', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTPPC', 'Twisting - PPC', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTPPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTPROD', 'Twisting - Production', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTPROD') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTWTRND', 'Twisting - Research and Development', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TWT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TWTRND') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTEXPT', 'Texturising - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTGEN', 'Texturising - General', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTGEN') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTMECH', 'Texturising - Mechanic', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTMECH') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTPACK', 'Texturising - Packing', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTPACK') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTPPC', 'Texturising - PPC', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTPPC') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOTXTPROD', 'Texturising - Production', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'TXT') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'TXTPROD') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
INSERT INTO mst_company_mapping (code, name, company_id, division_id, department_id, section_id, is_active, created_by)
SELECT 'MGTSLOUNHEXPT', 'Unit Head - Expatriat', co.company_id, dv.division_id, dpt.department_id, sec.section_id, true, 'seed'
FROM (SELECT company_id FROM mst_company WHERE company_code = 'MGT') co
CROSS JOIN (SELECT division_id FROM mst_division WHERE division_code = 'SLO') dv
CROSS JOIN (SELECT department_id FROM mst_department WHERE department_code = 'UNH') dpt
CROSS JOIN (SELECT section_id FROM mst_section WHERE section_code = 'UNHEXPT') sec
WHERE NOT EXISTS (
    SELECT 1 FROM mst_company_mapping m WHERE m.deleted_at IS NULL
      AND m.company_id = co.company_id AND m.division_id = dv.division_id
      AND m.department_id = dpt.department_id
      AND COALESCE(m.section_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(sec.section_id, '00000000-0000-0000-0000-000000000000'::uuid)
)
ON CONFLICT (code) DO NOTHING;
