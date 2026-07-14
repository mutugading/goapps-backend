package mbbatch

import "github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"

// The 7 F_MB_* formula codes and their result param codes, per migration
// 000452_seed_mst_formula_mb.up.sql (the definitive source — note F_MB_FIXED_COST's result
// param code is MB_FIXED_TOTAL, not MB_FIXED_COST; a deliberate divergence in the seed data).
const (
	FormulaCodeRMCost    = "F_MB_RM_COST"
	FormulaCodeWasteVal  = "F_MB_WASTE_VAL"
	FormulaCodeNetProd   = "F_MB_NET_PROD"
	FormulaCodeFixedCost = "F_MB_FIXED_COST"
	FormulaCodeOthers    = "F_MB_COST_OTHERS"
	FormulaCodeConvCost  = "F_MB_CONV_COST"
	FormulaCodeFinalCost = "F_MB_FINAL_COST"

	ResultParamRMCost    = "MB_RM_COST"
	ResultParamWasteVal  = "MB_WASTE_VAL"
	ResultParamNetProd   = "MB_NET_PROD"
	ResultParamFixedCost = "MB_FIXED_TOTAL"
	ResultParamOthers    = "MB_COST_OTHERS"
	ResultParamConvCost  = "MB_CONV_COST"
	ResultParamFinalCost = "MB_FINAL_COST"
)

// sharedFormulaCodes are computed once per MB, anchored to the ACTUAL calc type's
// MB_RM_COST, then reused (via CAPP pre-seeding) for the SELLING/FORECAST passes — PRD §8.1/§8.3.
var sharedFormulaCodes = map[string]struct{}{
	FormulaCodeWasteVal:  {},
	FormulaCodeNetProd:   {},
	FormulaCodeFixedCost: {},
	FormulaCodeOthers:    {},
	FormulaCodeConvCost:  {},
}

// sharedResultParamCodes mirrors sharedFormulaCodes, keyed by ResultParamCode — the set that
// must be pre-seeded into CAPP for the SELLING/FORECAST ComputeProduct calls.
var sharedResultParamCodes = map[string]struct{}{
	ResultParamWasteVal:  {},
	ResultParamNetProd:   {},
	ResultParamFixedCost: {},
	ResultParamOthers:    {},
	ResultParamConvCost:  {},
}

// partitionFormulas splits an MB's 7 topo-sorted formulas into the SHARED subset (computed once,
// anchored to ACTUAL) and the PER_TYPE subset (F_MB_RM_COST, F_MB_FINAL_COST — recomputed for
// every calc type). Relative order within each subset is preserved from the input (already
// topo-sorted by the loader; partitioning must never reorder).
func partitionFormulas(formulas []costcalc.Formula) (shared, perType []costcalc.Formula) {
	for _, f := range formulas {
		if _, ok := sharedFormulaCodes[f.FormulaCode]; ok {
			shared = append(shared, f)
			continue
		}
		perType = append(perType, f)
	}
	return shared, perType
}

// sharedOutputs extracts the SHARED formulas' computed values from a ParamSnapshot (the ACTUAL
// pass's ComputeOutput), for pre-seeding into the SELLING/FORECAST passes' CAPP maps.
func sharedOutputs(snapshot map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(sharedResultParamCodes))
	for code := range sharedResultParamCodes {
		if v, ok := snapshot[code]; ok {
			out[code] = v
		}
	}
	return out
}
