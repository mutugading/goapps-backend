-- Widen formula_type CHECK to add MB_COST_LOOKUP (12th value) — used only by the downstream
-- POY-side consumer formula reading cst_mb_cost, NOT by the 7 F_MB_* formulas themselves.
ALTER TABLE mst_formula DROP CONSTRAINT IF EXISTS mst_formula_formula_type_check;
ALTER TABLE mst_formula ADD CONSTRAINT mst_formula_formula_type_check
  CHECK (formula_type IN (
    'CALCULATION','SQL_QUERY','CONSTANT','LOOKUP','RM_LOOKUP','CONDITIONAL',
    'FROM_MARKETING','INTERMINGLING','SNAPSHOT','PENDING','INITIAL_VALUE',
    'MB_COST_LOOKUP'
  ));
