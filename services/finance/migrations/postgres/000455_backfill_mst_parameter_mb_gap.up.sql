-- Migration 000444 was edited in place (per its own 2026-07-11 addendum comments) to add
-- IS_BOUGHTOUT/MACHINE_MB_FIXED_TOTAL/MB_COMPOSITION_VERSION to mst_parameter AFTER it had
-- already been applied to this dev database. `migrate up` treats 000444 as already-run and
-- will never re-execute its body, so these 3 rows never landed here, breaking
-- mb_autogen_repository.go's mbResolveParamID lookup at Validate time
-- ("mb_autogen: resolve mst_parameter IS_BOUGHTOUT: sql: no rows in result set"). Backfill them
-- via a new migration instead of relying on 000444's ON CONFLICT DO NOTHING re-running.
INSERT INTO mst_parameter (param_code, param_name, data_type, param_category, created_by) VALUES
('IS_BOUGHTOUT','Is Bought-Out','BOOLEAN','INPUT','SYSTEM'),
('MACHINE_MB_FIXED_TOTAL','Machine MB Fixed Total','NUMBER','INPUT','SYSTEM'),
('MB_COMPOSITION_VERSION','MB Composition Version','NUMBER','INPUT','SYSTEM')
ON CONFLICT (param_code) WHERE deleted_at IS NULL DO NOTHING;
