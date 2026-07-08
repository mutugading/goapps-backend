-- Revert cpr classification constraints to their exact original definitions
-- from 000202_create_cost_product_request.up.sql.

ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_verified_override;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_verified_override
    CHECK (
        cpr_verified_classification IS NULL
        OR cpr_verified_classification = cpr_product_classification
        OR cpr_classification_override_reason IS NOT NULL
    );

ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_classification;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_classification
    CHECK (cpr_product_classification IN ('existing', 'new'));
