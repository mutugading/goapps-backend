// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// CstMBCostRepository implements persistence for the cst_mb_cost periodic active-cost cache —
// the only table downstream MB-cost consumers (POY, etc.) ever read from.
type CstMBCostRepository struct {
	db *DB
}

// NewCstMBCostRepository creates a new CstMBCostRepository instance.
func NewCstMBCostRepository(db *DB) *CstMBCostRepository {
	return &CstMBCostRepository{db: db}
}

// Upsert writes one cst_mb_cost row per (mbh_id, period, cost_type), called only from
// Push-to-Head execute — this table is never written from any other code path.
func (r *CstMBCostRepository) Upsert(ctx context.Context, tx *sql.Tx, mbhID, period, costType, costValue string, sourceCpcID int64, pushedBy string) error {
	const q = `
		INSERT INTO cst_mb_cost
			(mbc_mbh_id, mbc_period, mbc_cost_type, mbc_cost_value, mbc_source_cpc_id, mbc_pushed_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6)
		ON CONFLICT (mbc_mbh_id, mbc_period, mbc_cost_type)
		DO UPDATE SET mbc_cost_value = EXCLUDED.mbc_cost_value,
		              mbc_source_cpc_id = EXCLUDED.mbc_source_cpc_id,
		              mbc_pushed_at = NOW(),
		              mbc_pushed_by = EXCLUDED.mbc_pushed_by,
		              mbc_updated_at = NOW()`
	_, err := tx.ExecContext(ctx, q, mbhID, period, costType, costValue, sourceCpcID, pushedBy)
	if err != nil {
		return fmt.Errorf("cst_mb_cost_repository: upsert: %w", err)
	}
	return nil
}

// LatestByType returns the most recent active cost_value for mbhID + costType, used by
// Plan 04's LoadMBCosts calc-engine loader — the sole read path for MB cost consumers.
func (r *CstMBCostRepository) LatestByType(ctx context.Context, mbhID, costType string) (string, error) {
	const q = `
		SELECT mbc_cost_value FROM cst_mb_cost
		WHERE mbc_mbh_id = $1 AND mbc_cost_type = $2 AND mbc_is_active = TRUE
		ORDER BY mbc_period DESC LIMIT 1`
	var value string
	err := r.db.QueryRowContext(ctx, q, mbhID, costType).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("cst_mb_cost_repository: latest by type: %w", err)
	}
	return value, nil
}
