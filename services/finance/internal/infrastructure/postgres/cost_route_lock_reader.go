package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// IsLinkedRouteLocked implements cppapp.RouteLockReader.
// It resolves the route linked to a CPR via cpr_linked_route_head_id, then checks
// whether that route's crh_routing_status equals 'LOCKED'.
// Returns false (not locked) when the CPR has no linked route yet.
func (r *CostRouteRepository) IsLinkedRouteLocked(ctx context.Context, requestID int64) (bool, error) {
	const q = `
SELECT COALESCE(h.crh_routing_status, '') = 'LOCKED'
FROM cost_product_request r
LEFT JOIN cost_route_head h ON h.crh_head_id = r.cpr_linked_route_head_id
WHERE r.cpr_request_id = $1`

	var locked bool
	err := r.db.QueryRowContext(ctx, q, requestID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check linked route lock status: %w", err)
	}
	return locked, nil
}
