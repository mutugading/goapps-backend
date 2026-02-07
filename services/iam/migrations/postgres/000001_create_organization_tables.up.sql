-- IAM Service Database Migrations
-- 000001: Create organization hierarchy tables
--
-- Tables: mst_company, mst_division, mst_department, mst_section
-- These tables store the organization structure hierarchy

-- Enable UUID extension if not exists
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- For trigram search

-- =============================================================================
-- COMPANY TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_company (
    company_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_code VARCHAR(20) NOT NULL,
    company_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT uq_company_code UNIQUE (company_code),
    CONSTRAINT chk_company_code_format CHECK (company_code ~ '^[A-Z][A-Z0-9_]*$')
);

-- Company indexes
CREATE INDEX idx_company_code ON mst_company(company_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_name ON mst_company(company_name) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_active ON mst_company(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_company_search ON mst_company USING gin(
    (company_code || ' ' || company_name) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_company IS 'Master table for company/organization entities';
COMMENT ON COLUMN mst_company.company_code IS 'Unique company code, e.g., MGT (Mutu Gading)';

-- =============================================================================
-- DIVISION TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_division (
    division_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id UUID NOT NULL,
    division_code VARCHAR(20) NOT NULL,
    division_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT fk_division_company FOREIGN KEY (company_id) REFERENCES mst_company(company_id) ON DELETE RESTRICT,
    CONSTRAINT uq_division_code UNIQUE (division_code),
    CONSTRAINT chk_division_code_format CHECK (division_code ~ '^[A-Z][A-Z0-9_]*$')
);

-- Division indexes
CREATE INDEX idx_division_company ON mst_division(company_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_division_code ON mst_division(division_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_division_active ON mst_division(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_division_search ON mst_division USING gin(
    (division_code || ' ' || division_name) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_division IS 'Master table for divisions under company';
COMMENT ON COLUMN mst_division.division_code IS 'Unique division code, e.g., SLO (Solo)';

-- =============================================================================
-- DEPARTMENT TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_department (
    department_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    division_id UUID NOT NULL,
    department_code VARCHAR(20) NOT NULL,
    department_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT fk_department_division FOREIGN KEY (division_id) REFERENCES mst_division(division_id) ON DELETE RESTRICT,
    CONSTRAINT uq_department_code UNIQUE (department_code),
    CONSTRAINT chk_department_code_format CHECK (department_code ~ '^[A-Z][A-Z0-9_]*$')
);

-- Department indexes
CREATE INDEX idx_department_division ON mst_department(division_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_department_code ON mst_department(department_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_department_active ON mst_department(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_department_search ON mst_department USING gin(
    (department_code || ' ' || department_name) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_department IS 'Master table for departments under division';
COMMENT ON COLUMN mst_department.department_code IS 'Unique department code, e.g., IT (Information Technology)';

-- =============================================================================
-- SECTION TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_section (
    section_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    department_id UUID NOT NULL,
    section_code VARCHAR(20) NOT NULL,
    section_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT fk_section_department FOREIGN KEY (department_id) REFERENCES mst_department(department_id) ON DELETE RESTRICT,
    CONSTRAINT uq_section_code UNIQUE (section_code),
    CONSTRAINT chk_section_code_format CHECK (section_code ~ '^[A-Z][A-Z0-9_]*$')
);

-- Section indexes
CREATE INDEX idx_section_department ON mst_section(department_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_section_code ON mst_section(section_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_section_active ON mst_section(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_section_search ON mst_section USING gin(
    (section_code || ' ' || section_name) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_section IS 'Master table for sections under department';
COMMENT ON COLUMN mst_section.section_code IS 'Unique section code, e.g., SOFT (Software)';
