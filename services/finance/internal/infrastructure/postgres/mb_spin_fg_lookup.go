package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

var _ mbsource.SpinShadeFgFinder = (*MBSpinRepository)(nil)

// FindByShadeAndFg returns, per (normalized shade, FG product id) key, the live + active +
// non-superseded MB spin linked to that FG product (mb-spin-fg-poy-link P4). One batched
// query for any number of keys; empty input runs no query. Legacy spins without an FG link
// are never returned. Implements mbsource.SpinShadeFgFinder.
func (r *MBSpinRepository) FindByShadeAndFg(ctx context.Context, keys []mbsource.ShadeFgKey) (map[mbsource.ShadeFgKey]*mbspin.Entity, error) {
	out := make(map[mbsource.ShadeFgKey]*mbspin.Entity, len(keys))
	shades := make([]string, 0, len(keys))
	fgs := make([]int64, 0, len(keys))
	for _, k := range keys {
		s := mbsource.NormalizeShade(k.Shade)
		if s == "" || k.FgProductID <= 0 {
			continue
		}
		shades = append(shades, s)
		fgs = append(fgs, k.FgProductID)
	}
	if len(shades) == 0 {
		return out, nil
	}
	q := r.selectCols() + `
		WHERE deleted_at IS NULL AND mbs_is_active
		  AND mbs_superseded_by_spin_id IS NULL
		  AND mbs_fg_product_id IS NOT NULL
		  AND (UPPER(TRIM(mbs_shade_code)), mbs_fg_product_id) IN (
			SELECT k.shade, k.fg FROM unnest(CAST($1 AS TEXT[]), CAST($2 AS BIGINT[])) AS k(shade, fg))
		ORDER BY mbs_id`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(shades), pq.Array(fgs))
	if err != nil {
		return nil, fmt.Errorf("find mb spins by shade and fg: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sc, fg := e.ShadeCode(), e.FgProductID()
		if sc == nil || fg == nil {
			continue
		}
		key := mbsource.ShadeFgKey{Shade: mbsource.NormalizeShade(*sc), FgProductID: *fg}
		if _, dup := out[key]; !dup {
			out[key] = e
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mb spins by shade and fg: %w", err)
	}
	return out, nil
}
