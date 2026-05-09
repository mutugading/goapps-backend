-- Migration: Create cst_product table for Product Costing.
-- Phase 1: only DRAFT workflow_status used; later phases populate other states.

CREATE TABLE IF NOT EXISTS cst_product (
    product_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_code            VARCHAR(30) NOT NULL,
    product_name            VARCHAR(200) NOT NULL,
    product_item_code       VARCHAR(30) NOT NULL,
    product_shade_code      VARCHAR(30),
    product_shade_name      VARCHAR(100),
    product_status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
    workflow_status         VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
    created_by_dept_id      UUID,
    created_by_dept_code    VARCHAR(10),
    purpose                 VARCHAR(20) NOT NULL DEFAULT 'COMMERCIAL',
    duplicated_from_id      UUID REFERENCES cst_product(product_id),
    duplication_note        VARCHAR(500),
    copied_with_options     JSONB,
    template_id             UUID,
    template_version_pinned INT,
    current_request_id      UUID,
    locked_at               TIMESTAMP,
    locked_by               VARCHAR(100),
    locked_period           VARCHAR(20),
    unlock_count            INT NOT NULL DEFAULT 0,
    created_at              TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by              VARCHAR(100) NOT NULL,
    updated_at              TIMESTAMP,
    updated_by              VARCHAR(100),
    deleted_at              TIMESTAMP,
    deleted_by              VARCHAR(100),
    CONSTRAINT chk_cst_product_status CHECK (product_status IN ('DRAFT','PARAM_PENDING','ACTIVE','INACTIVE')),
    CONSTRAINT chk_cst_product_workflow CHECK (workflow_status IN ('DRAFT','SUBMITTED','CONFIRMED','LOCKED','UNLOCK_REQUESTED')),
    CONSTRAINT chk_cst_product_purpose CHECK (purpose IN ('COMMERCIAL','TESTING','TRIAL'))
);

-- Unique constraints (partial — only on non-soft-deleted rows so soft-deleted codes can be reused)
CREATE UNIQUE INDEX IF NOT EXISTS uk_cst_product_code      ON cst_product(product_code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uk_cst_product_item_code ON cst_product(product_item_code) WHERE deleted_at IS NULL;

-- Filter / lookup indexes
CREATE INDEX IF NOT EXISTS idx_cst_product_workflow ON cst_product(workflow_status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cst_product_dept     ON cst_product(created_by_dept_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cst_product_template ON cst_product(template_id, template_version_pinned);
CREATE INDEX IF NOT EXISTS idx_cst_product_request  ON cst_product(current_request_id);
CREATE INDEX IF NOT EXISTS idx_cst_product_duplicated_from ON cst_product(duplicated_from_id);

-- Full-text search for "match existing product" by name/shade/code
CREATE INDEX IF NOT EXISTS idx_cst_product_fts ON cst_product
    USING gin(to_tsvector('simple',
        coalesce(product_name,'') || ' ' ||
        coalesce(product_shade_name,'') || ' ' ||
        coalesce(product_code,'')))
    WHERE deleted_at IS NULL;

COMMENT ON TABLE cst_product IS 'Product master for costing. Aggregate root.';
COMMENT ON COLUMN cst_product.workflow_status IS 'DRAFT/SUBMITTED/CONFIRMED/LOCKED/UNLOCK_REQUESTED — lifecycle state for cost approval workflow.';
COMMENT ON COLUMN cst_product.copied_with_options IS 'JSON of {include_values, include_routing, include_rm, include_attachments} when this product was duplicated.';
COMMENT ON COLUMN cst_product.template_id IS 'FK to cst_product_template (table created in P7). NULL = not derived from template.';
COMMENT ON COLUMN cst_product.template_version_pinned IS 'Specific version number from cst_product_template_version (P7).';
