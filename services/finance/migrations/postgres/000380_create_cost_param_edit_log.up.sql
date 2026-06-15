-- 000380: audit log for param value overrides.
-- Records every time an authorized user manually overrides a param value outside
-- the normal fill-task flow. Used to display "Last edited by" in the UI.

CREATE TABLE IF NOT EXISTS cost_param_edit_log (
    cpel_id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cpel_request_id  BIGINT      NOT NULL,
    cpel_route_level INT         NOT NULL,
    cpel_param_code  VARCHAR(100) NOT NULL,
    cpel_old_value   TEXT,
    cpel_new_value   TEXT,
    cpel_changed_by  VARCHAR(36) NOT NULL,
    cpel_changed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cost_param_edit_log_request_level
    ON cost_param_edit_log (cpel_request_id, cpel_route_level, cpel_changed_at DESC);
