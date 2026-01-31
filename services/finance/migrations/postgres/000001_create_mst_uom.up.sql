-- Migration: Create mst_uom table
-- Description: Master table for Units of Measure

CREATE TABLE IF NOT EXISTS mst_uom (
    -- Primary Key
    uom_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Core Fields
    uom_code VARCHAR(20) NOT NULL,
    uom_name VARCHAR(100) NOT NULL,
    uom_category VARCHAR(50) NOT NULL,
    description TEXT,
    
    -- Status
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    
    -- Audit Fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),
    
    -- Soft Delete Fields
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(100),
    
    -- Constraints
    CONSTRAINT uq_mst_uom_code UNIQUE (uom_code),
    CONSTRAINT chk_mst_uom_category CHECK (uom_category IN ('WEIGHT', 'LENGTH', 'VOLUME', 'QUANTITY'))
);

-- Comments
COMMENT ON TABLE mst_uom IS 'Master table for Units of Measure';
COMMENT ON COLUMN mst_uom.uom_id IS 'Unique identifier (UUID)';
COMMENT ON COLUMN mst_uom.uom_code IS 'Unique code (e.g., KG, MTR, PCS)';
COMMENT ON COLUMN mst_uom.uom_name IS 'Display name (e.g., Kilogram, Meter)';
COMMENT ON COLUMN mst_uom.uom_category IS 'Category: WEIGHT, LENGTH, VOLUME, QUANTITY';
COMMENT ON COLUMN mst_uom.is_active IS 'Whether the UOM is active';
COMMENT ON COLUMN mst_uom.deleted_at IS 'Soft delete timestamp (NULL = not deleted)';

-- Indexes for common queries (excluding soft-deleted records)
CREATE INDEX idx_mst_uom_code ON mst_uom(uom_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_mst_uom_category ON mst_uom(uom_category) WHERE deleted_at IS NULL;
CREATE INDEX idx_mst_uom_active ON mst_uom(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_mst_uom_created_at ON mst_uom(created_at) WHERE deleted_at IS NULL;

-- Full-text search index for search functionality
CREATE INDEX idx_mst_uom_search ON mst_uom 
    USING gin(to_tsvector('english', coalesce(uom_code, '') || ' ' || coalesce(uom_name, '') || ' ' || coalesce(description, ''))) 
    WHERE deleted_at IS NULL;
