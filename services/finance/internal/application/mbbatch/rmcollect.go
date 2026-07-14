package mbbatch

import costroutedom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"

// collectGroupCodes returns the deduped rm_group_code strings referenced by GROUP-type RMs in a
// single MB's route. Task 20b's auto-gen'd MB route only ever contains GROUP and PRODUCT RM
// types (never ITEM), so unlike costcalc's collectRMCodes this never needs to check RmTypeItem.
func collectGroupCodes(route *costroutedom.Graph) []string {
	if route == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, seq := range route.Seqs {
		if seq == nil {
			continue
		}
		for _, rm := range seq.Rms {
			if rm == nil || rm.RmType != costroutedom.RmTypeGroup {
				continue
			}
			if _, ok := seen[rm.RmGroupCode]; !ok {
				seen[rm.RmGroupCode] = struct{}{}
				out = append(out, rm.RmGroupCode)
			}
		}
	}
	return out
}

// collectNestedMBProducts returns the deduped upstream product sys IDs referenced by
// PRODUCT-type RMs in a single MB's route (nested-MB composition references).
func collectNestedMBProducts(route *costroutedom.Graph) []int64 {
	if route == nil {
		return nil
	}
	seen := map[int64]struct{}{}
	out := []int64{}
	for _, seq := range route.Seqs {
		if seq == nil {
			continue
		}
		for _, rm := range seq.Rms {
			if rm == nil || rm.RmType != costroutedom.RmTypeProduct {
				continue
			}
			if _, ok := seen[rm.RmProductSysID]; !ok {
				seen[rm.RmProductSysID] = struct{}{}
				out = append(out, rm.RmProductSysID)
			}
		}
	}
	return out
}
