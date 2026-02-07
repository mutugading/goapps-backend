-- IAM Service Database Migrations
-- 000004: Create RBAC tables
--
-- Tables: mst_role, mst_permission, user_roles, user_permissions, role_permissions
-- Role-based access control with multi-role and direct permission support

-- =============================================================================
-- ROLE TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_role (
    role_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role_code VARCHAR(50) NOT NULL,
    role_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT false,  -- System roles cannot be deleted
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT uq_role_code UNIQUE (role_code),
    CONSTRAINT chk_role_code_format CHECK (role_code ~ '^[A-Z][A-Z0-9_]*$')
);

-- Role indexes
CREATE INDEX idx_role_code ON mst_role(role_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_role_active ON mst_role(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_role_system ON mst_role(is_system) WHERE deleted_at IS NULL;
CREATE INDEX idx_role_search ON mst_role USING gin(
    (role_code || ' ' || role_name) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_role IS 'Master table for role definitions';
COMMENT ON COLUMN mst_role.is_system IS 'System roles (SUPER_ADMIN, etc.) cannot be deleted';

-- =============================================================================
-- PERMISSION TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_permission (
    permission_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    permission_code VARCHAR(100) NOT NULL,  -- e.g., finance.master.uom.view
    permission_name VARCHAR(100) NOT NULL,
    description TEXT,
    service_name VARCHAR(50) NOT NULL,  -- finance, hr, iam, etc.
    module_name VARCHAR(50) NOT NULL,   -- master, transaction, report, etc.
    action_type VARCHAR(20) NOT NULL,   -- view, create, update, delete, export, import
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    CONSTRAINT uq_permission_code UNIQUE (permission_code),
    CONSTRAINT chk_permission_code_format CHECK (permission_code ~ '^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$'),
    CONSTRAINT chk_permission_action CHECK (action_type IN ('view', 'create', 'update', 'delete', 'export', 'import'))
);

-- Permission indexes
CREATE INDEX idx_permission_code ON mst_permission(permission_code) WHERE is_active = true;
CREATE INDEX idx_permission_service ON mst_permission(service_name) WHERE is_active = true;
CREATE INDEX idx_permission_module ON mst_permission(service_name, module_name) WHERE is_active = true;
CREATE INDEX idx_permission_action ON mst_permission(action_type) WHERE is_active = true;
CREATE INDEX idx_permission_search ON mst_permission USING gin(
    (permission_code || ' ' || permission_name) gin_trgm_ops
) WHERE is_active = true;

COMMENT ON TABLE mst_permission IS 'Master table for granular permissions';
COMMENT ON COLUMN mst_permission.permission_code IS 'Format: {service}.{module}.{entity}.{action}';

-- =============================================================================
-- USER ROLES TABLE (Many-to-Many: Users <-> Roles)
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    role_id UUID NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(100) NOT NULL,
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES mst_role(role_id) ON DELETE CASCADE,
    CONSTRAINT uq_user_role UNIQUE (user_id, role_id)
);

-- User roles indexes
CREATE INDEX idx_user_roles_user ON user_roles(user_id);
CREATE INDEX idx_user_roles_role ON user_roles(role_id);

COMMENT ON TABLE user_roles IS 'Join table for user-role assignments (1 user can have multiple roles)';

-- =============================================================================
-- USER PERMISSIONS TABLE (Direct permission assignment without going through roles)
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(100) NOT NULL,
    CONSTRAINT fk_user_permissions_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_user_permissions_permission FOREIGN KEY (permission_id) REFERENCES mst_permission(permission_id) ON DELETE CASCADE,
    CONSTRAINT uq_user_permission UNIQUE (user_id, permission_id)
);

-- User permissions indexes
CREATE INDEX idx_user_permissions_user ON user_permissions(user_id);
CREATE INDEX idx_user_permissions_permission ON user_permissions(permission_id);

COMMENT ON TABLE user_permissions IS 'Direct permission assignments (overrides role-based permissions)';

-- =============================================================================
-- ROLE PERMISSIONS TABLE (Many-to-Many: Roles <-> Permissions)
-- =============================================================================
CREATE TABLE IF NOT EXISTS role_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role_id UUID NOT NULL,
    permission_id UUID NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by VARCHAR(100) NOT NULL,
    CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES mst_role(role_id) ON DELETE CASCADE,
    CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id) REFERENCES mst_permission(permission_id) ON DELETE CASCADE,
    CONSTRAINT uq_role_permission UNIQUE (role_id, permission_id)
);

-- Role permissions indexes
CREATE INDEX idx_role_permissions_role ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission ON role_permissions(permission_id);

COMMENT ON TABLE role_permissions IS 'Join table for role-permission assignments';
