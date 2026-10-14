-- [T003] Jalankan ulang 04§11 untuk objek yang dibuat setelahnya (§14–§18), sesuai catatan spec.
GRANT SELECT ON ALL TABLES IN SCHEMA payroll TO payroll_ro;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payroll TO payroll_app;
REVOKE UPDATE, DELETE ON payroll.pyr_audit_log FROM payroll_app;
REVOKE UPDATE, DELETE, INSERT ON payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_rule_component FROM payroll_app;
GRANT SELECT ON payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_rule_component TO payroll_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_app;
