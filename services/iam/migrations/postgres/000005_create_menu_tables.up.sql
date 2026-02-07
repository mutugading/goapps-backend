-- IAM Service Database Migrations
-- 000005: Create menu tables
--
-- Tables: mst_menu, menu_permissions
-- Dynamic 3-level menu with icons and permission control

-- =============================================================================
-- MENU TABLE (3-Level Hierarchy)
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_menu (
    menu_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parent_id UUID,  -- Null for root level menus
    menu_code VARCHAR(50) NOT NULL,
    menu_title VARCHAR(100) NOT NULL,
    menu_url VARCHAR(255),  -- Null for parent-only menus without direct links
    icon_name VARCHAR(50),  -- Lucide icon name (e.g., DollarSign, Users, Settings)
    service_name VARCHAR(50) NOT NULL,  -- finance, hr, iam, etc.
    menu_level INTEGER NOT NULL,  -- 1=root, 2=parent, 3=child
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_visible BOOLEAN NOT NULL DEFAULT true,  -- Show in sidebar
    is_active BOOLEAN NOT NULL DEFAULT true,   -- Active/inactive toggle
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT fk_menu_parent FOREIGN KEY (parent_id) REFERENCES mst_menu(menu_id) ON DELETE SET NULL,
    CONSTRAINT uq_menu_code UNIQUE (menu_code),
    CONSTRAINT chk_menu_code_format CHECK (menu_code ~ '^[A-Z][A-Z0-9_]*$'),
    CONSTRAINT chk_menu_level CHECK (menu_level BETWEEN 1 AND 3),
    -- Root menus must not have a parent
    CONSTRAINT chk_root_no_parent CHECK (menu_level != 1 OR parent_id IS NULL),
    -- Non-root menus must have a parent
    CONSTRAINT chk_child_has_parent CHECK (menu_level = 1 OR parent_id IS NOT NULL)
);

-- Menu indexes
CREATE INDEX idx_menu_parent ON mst_menu(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_code ON mst_menu(menu_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_service ON mst_menu(service_name) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_level ON mst_menu(menu_level) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_sort ON mst_menu(parent_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_visible ON mst_menu(is_visible) WHERE deleted_at IS NULL AND is_active = true;
CREATE INDEX idx_menu_search ON mst_menu USING gin(
    (menu_code || ' ' || menu_title) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_menu IS 'Dynamic menu configuration with 3-level hierarchy';
COMMENT ON COLUMN mst_menu.icon_name IS 'Lucide icon name for sidebar display';
COMMENT ON COLUMN mst_menu.menu_level IS '1=root, 2=parent, 3=child (max 3 levels)';

-- =============================================================================
-- MENU PERMISSIONS TABLE (Many-to-Many: Menus <-> Permissions)
-- =============================================================================
CREATE TABLE IF NOT EXISTS menu_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    menu_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(100) NOT NULL,
    CONSTRAINT fk_menu_permissions_menu FOREIGN KEY (menu_id) REFERENCES mst_menu(menu_id) ON DELETE CASCADE,
    CONSTRAINT fk_menu_permissions_permission FOREIGN KEY (permission_id) REFERENCES mst_permission(permission_id) ON DELETE CASCADE,
    CONSTRAINT uq_menu_permission UNIQUE (menu_id, permission_id)
);

-- Menu permissions indexes
CREATE INDEX idx_menu_permissions_menu ON menu_permissions(menu_id);
CREATE INDEX idx_menu_permissions_permission ON menu_permissions(permission_id);

COMMENT ON TABLE menu_permissions IS 'Join table for menu-permission requirements (user needs ANY of these permissions to see menu)';
