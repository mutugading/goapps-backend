-- Add UOM categories needed for costing parameters (CURRENCY, DIMENSIONLESS)
INSERT INTO mst_uom_category (uom_category_id, category_code, category_name, description, created_by)
VALUES
  (gen_random_uuid(), 'CURRENCY',      'Currency',      'Currency units (USD, IDR)',          'system'),
  (gen_random_uuid(), 'DIMENSIONLESS', 'Dimensionless', 'Ratio and percentage units',         'system')
ON CONFLICT (category_code) DO NOTHING;

-- Add UOM codes referenced by mst_parameter Excel data
INSERT INTO mst_uom (uom_id, uom_code, uom_name, uom_category_id, created_by)
SELECT gen_random_uuid(), 'KGS', 'Kilograms', c.uom_category_id, 'system'
  FROM mst_uom_category c WHERE c.category_code = 'WEIGHT'
ON CONFLICT (uom_code) DO NOTHING;

INSERT INTO mst_uom (uom_id, uom_code, uom_name, uom_category_id, created_by)
SELECT gen_random_uuid(), 'USD', 'US Dollar', c.uom_category_id, 'system'
  FROM mst_uom_category c WHERE c.category_code = 'CURRENCY'
ON CONFLICT (uom_code) DO NOTHING;

INSERT INTO mst_uom (uom_id, uom_code, uom_name, uom_category_id, created_by)
SELECT gen_random_uuid(), 'PERCENTAGE', 'Percentage', c.uom_category_id, 'system'
  FROM mst_uom_category c WHERE c.category_code = 'DIMENSIONLESS'
ON CONFLICT (uom_code) DO NOTHING;
