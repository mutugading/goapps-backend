-- 000433: add free node positions to cost_route_rm so RM nodes on the routing
-- graph editor can be dragged anywhere and persist (matching cost_route_seq's
-- crs_position_x/y). Nullable: existing rows keep NULL and the editor falls back
-- to auto-layout until the user drags them.

ALTER TABLE cost_route_rm
    ADD COLUMN IF NOT EXISTS crm_position_x DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS crm_position_y DOUBLE PRECISION;
