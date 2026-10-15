-- H004 / HD-10: permission BI Workforce + role Direksi/GM & HR.
-- workforce.view         = akses report Workforce (level departemen).
-- workforceemployee.view = data level karyawan (NIK di group_3, flag sakit); ditegakkan di server (HD-07).
-- Role dibuat bertanda description '[H004]' supaya down hanya menghapus role milik migrasi ini.
-- Kepala dept ditunda ke fase 2 (HD-11). Idempotent.

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.bi.workforce.view',         'View BI Workforce',          'Lihat report Workforce (OT, sakit, disiplin) level departemen',         'finance', 'bi', 'view', true, 'seed'),
    ('finance.bi.workforceemployee.view', 'View BI Workforce Employee', 'Lihat data level karyawan (NIK/group_3) pada report Workforce',         'finance', 'bi', 'view', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by)
VALUES
    (gen_random_uuid(), 'DIREKSI_GM', 'Direksi / GM', '[H004] Report Workforce level departemen',                      FALSE, TRUE, 'seed'),
    (gen_random_uuid(), 'HR',         'HR',           '[H004] Report Workforce termasuk level karyawan & flag sakit', FALSE, TRUE, 'seed')
ON CONFLICT DO NOTHING;

-- Kedua role: dashboard.view + workforce.view.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code IN ('DIREKSI_GM', 'HR') AND r.is_active = true
  AND p.permission_code IN ('finance.bi.dashboard.view', 'finance.bi.workforce.view') AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Level karyawan: hanya HR (PRD §7).
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'HR' AND r.is_active = true
  AND p.permission_code = 'finance.bi.workforceemployee.view' AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- SUPER_ADMIN: semua permission BI baru.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = true
  AND p.permission_code IN ('finance.bi.workforce.view', 'finance.bi.workforceemployee.view') AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
