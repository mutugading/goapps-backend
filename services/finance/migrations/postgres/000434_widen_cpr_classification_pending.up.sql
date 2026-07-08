-- Widen cost_product_request classification to allow an unclassified "pending"
-- placeholder value, and treat it as not requiring an override reason when
-- verified later (resolving a pending classification is not an override).

ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_classification;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_classification
    CHECK (cpr_product_classification IN ('existing', 'new', 'pending'));

ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_verified_override;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_verified_override
    CHECK (
        cpr_verified_classification IS NULL
        OR cpr_verified_classification = cpr_product_classification
        OR cpr_product_classification = 'pending'
        OR cpr_classification_override_reason IS NOT NULL
    );
