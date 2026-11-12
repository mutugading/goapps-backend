package mbsource

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// ShadeFgKey identifies one FG-linked MB spin lookup: a normalized shade (NormalizeShade)
// paired with the FG (POY) product id the spin is linked to.
type ShadeFgKey struct {
	// Shade is the normalized (UPPER(TRIM)) shade code.
	Shade string
	// FgProductID is the cost_product_master id of the FG (POY) product.
	FgProductID int64
}

// SpinShadeFgFinder returns, per (shade, FG product) key, the single live, active,
// non-superseded MB spin linked to that FG product (at most one by uix_mbs_shade_fg_live).
// Keys without a match are absent. Input shades are normalized by the implementation.
type SpinShadeFgFinder interface {
	FindByShadeAndFg(ctx context.Context, keys []ShadeFgKey) (map[ShadeFgKey]*mbspin.Entity, error)
}

// FgCandidateFinder returns, per product, the ordered FG (POY) candidate product ids: the
// product itself when it is POY, else the upstream POY RM products of its active route
// (ordered by route level, route seq, RM id; first occurrence kept). Products without any
// candidate are absent.
type FgCandidateFinder interface {
	FgCandidates(ctx context.Context, productSysIDs []int64) (map[int64][]int64, error)
}
