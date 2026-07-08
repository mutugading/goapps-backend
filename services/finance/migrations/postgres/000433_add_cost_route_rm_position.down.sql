-- Revert 000433: drop the RM node position columns.

ALTER TABLE cost_route_rm
    DROP COLUMN IF EXISTS crm_position_x,
    DROP COLUMN IF EXISTS crm_position_y;
