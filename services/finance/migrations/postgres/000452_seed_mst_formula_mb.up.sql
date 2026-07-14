-- 7 new F_MB_* formula rows. F_MB_RM_COST needs dedicated Go computation code (Plan 04);
-- the other 6 are plain CALCULATION type, evaluated by the generic engine unchanged.
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, created_by)
SELECT v.formula_code, v.formula_name, v.formula_type, v.expression, p.id, 'SYSTEM'
FROM (VALUES
  ('F_MB_RM_COST', 'MB Raw Material Cost', 'RM_LOOKUP',
   'sum(RM_Cost[i]) for composition items i, WHERE RM_Cost[i] = (mbcm_source_type=''MB'' ? cst_product_cost.cpc_cost_per_unit[MB_ref,period,cost_type] : cst_rm_cost.<source_col_for(cost_type)>[group_head_id,period]) * Composition[i]/100 — per cost_type (ACTUAL/SELLING/FORECAST), Go-computed, not expr-lang',
   'MB_RM_COST'),
  ('F_MB_WASTE_VAL', 'MB Waste Value', 'CALCULATION',
   '(MB_RM_COST / (1 - MB_WASTE/100)) * (MB_WASTE/100)',
   'MB_WASTE_VAL'),
  ('F_MB_NET_PROD', 'MB Net Production', 'CALCULATION',
   'MB_THROUGHPUT * (MB_EFFICIENCY/100) * MB_PROD_PER_DAY',
   'MB_NET_PROD'),
  ('F_MB_FIXED_COST', 'MB Fixed Cost Total', 'CALCULATION',
   'MB_NET_PROD > 0 ? MACHINE_MB_FIXED_TOTAL / MB_NET_PROD : 0',
   'MB_FIXED_TOTAL'),
  ('F_MB_COST_OTHERS', 'MB Cost Others', 'CALCULATION',
   '((MB_RM_COST + MB_WASTE_VAL + MB_FIXED_TOTAL) * ((MB_QUALITY_LOSS + MB_DEV_EXPENSE)/100)) + MB_PACKING',
   'MB_COST_OTHERS'),
  ('F_MB_CONV_COST', 'MB Conversion Cost', 'CALCULATION',
   '(MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL',
   'MB_CONV_COST'),
  ('F_MB_FINAL_COST', 'MB Final Cost', 'CALCULATION',
   'IS_BOUGHTOUT == 1 ? MB_RM_COST : MB_RM_COST + MB_CONV_COST',
   'MB_FINAL_COST')
) AS v(formula_code, formula_name, formula_type, expression, result_param_code)
JOIN mst_parameter p ON p.param_code = v.result_param_code AND p.deleted_at IS NULL
ON CONFLICT (formula_code) WHERE deleted_at IS NULL DO NOTHING;

-- ============================================================
-- PART 2: Insert formula_param (input param links), same pattern as
-- 000408_seed_oracle_formulas.up.sql PART 2. Drives topoSortFormulas'
-- dependency ordering (loader.go) and buildInitialScope's zero-fill safety
-- (compute.go) — without these rows the 7 formulas above evaluate in
-- encounter order, which can read undefined/zero upstream values.
-- F_MB_RM_COST has no rows here: it is RM_LOOKUP type, aliased directly
-- from the pre-aggregated RM total in evalSingleFormulaStep, bypassing
-- expr-lang and InputParamCodes entirely. It still participates correctly
-- as an *input* to other formulas below via its ResultParamCode (MB_RM_COST).
-- ============================================================

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
-- F_MB_WASTE_VAL: MB_RM_COST, MB_WASTE
  ('F_MB_WASTE_VAL','MB_RM_COST',1),('F_MB_WASTE_VAL','MB_WASTE',2),
-- F_MB_NET_PROD: MB_THROUGHPUT, MB_EFFICIENCY, MB_PROD_PER_DAY
  ('F_MB_NET_PROD','MB_THROUGHPUT',1),('F_MB_NET_PROD','MB_EFFICIENCY',2),('F_MB_NET_PROD','MB_PROD_PER_DAY',3),
-- F_MB_FIXED_COST: MB_NET_PROD, MACHINE_MB_FIXED_TOTAL
  ('F_MB_FIXED_COST','MB_NET_PROD',1),('F_MB_FIXED_COST','MACHINE_MB_FIXED_TOTAL',2),
-- F_MB_COST_OTHERS: MB_RM_COST, MB_WASTE_VAL, MB_FIXED_TOTAL, MB_QUALITY_LOSS, MB_DEV_EXPENSE, MB_PACKING
  ('F_MB_COST_OTHERS','MB_RM_COST',1),('F_MB_COST_OTHERS','MB_WASTE_VAL',2),('F_MB_COST_OTHERS','MB_FIXED_TOTAL',3),
  ('F_MB_COST_OTHERS','MB_QUALITY_LOSS',4),('F_MB_COST_OTHERS','MB_DEV_EXPENSE',5),('F_MB_COST_OTHERS','MB_PACKING',6),
-- F_MB_CONV_COST: MB_NO_PROCESS, MB_FIXED_TOTAL, MB_COST_OTHERS, MB_WASTE_VAL
  ('F_MB_CONV_COST','MB_NO_PROCESS',1),('F_MB_CONV_COST','MB_FIXED_TOTAL',2),('F_MB_CONV_COST','MB_COST_OTHERS',3),('F_MB_CONV_COST','MB_WASTE_VAL',4),
-- F_MB_FINAL_COST: IS_BOUGHTOUT, MB_RM_COST, MB_CONV_COST
  ('F_MB_FINAL_COST','IS_BOUGHTOUT',1),('F_MB_FINAL_COST','MB_RM_COST',2),('F_MB_FINAL_COST','MB_CONV_COST',3)
) AS fp(fcode, pcode, sort_order)
WHERE
    (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND NOT EXISTS (
    SELECT 1 FROM formula_param fp2
    WHERE fp2.formula_id = (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1)
      AND fp2.param_id   = (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1)
);
