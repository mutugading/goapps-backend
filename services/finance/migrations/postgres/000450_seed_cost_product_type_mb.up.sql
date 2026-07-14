INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
VALUES ('MB', 'Master Batch')
ON CONFLICT (cpt_type_code) DO NOTHING;
