package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// LookupMasterRepository implements lookupmaster.Repository against PostgreSQL.
type LookupMasterRepository struct {
	db *DB
}

// NewLookupMasterRepository creates a new LookupMasterRepository.
func NewLookupMasterRepository(db *DB) *LookupMasterRepository {
	return &LookupMasterRepository{db: db}
}

// Verify interface implementation at compile time.
var _ lookupmaster.Repository = (*LookupMasterRepository)(nil)

// ListMasters returns lookup master records, optionally filtered to active only.
func (r *LookupMasterRepository) ListMasters(ctx context.Context, activeOnly bool) ([]*lookupmaster.LookupMaster, error) {
	q := `SELECT lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, lm_is_active
	      FROM mst_lookup_master`
	if activeOnly {
		q += ` WHERE lm_is_active = TRUE`
	}
	q += ` ORDER BY lm_code`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list lookup masters: %w", err)
	}

	var out []*lookupmaster.LookupMaster
	for rows.Next() {
		m := &lookupmaster.LookupMaster{}
		if scanErr := rows.Scan(&m.Code, &m.DisplayName, &m.APIPath, &m.CodeField, &m.LabelField, &m.IsActive); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan lookup master: %w", scanErr)
		}
		out = append(out, m)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close lookup master rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup masters: %w", err)
	}
	return out, nil
}

// ListColumns returns fillable columns for the given master code, sorted by sort_order.
func (r *LookupMasterRepository) ListColumns(ctx context.Context, masterCode string) ([]*lookupmaster.Column, error) {
	const q = `SELECT lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order
	           FROM mst_lookup_master_column
	           WHERE lmc_master_code = $1
	           ORDER BY lmc_sort_order, lmc_column_name`

	rows, err := r.db.QueryContext(ctx, q, masterCode)
	if err != nil {
		return nil, fmt.Errorf("list lookup master columns: %w", err)
	}

	var out []*lookupmaster.Column
	for rows.Next() {
		c := &lookupmaster.Column{}
		if scanErr := rows.Scan(&c.MasterCode, &c.ColumnName, &c.DisplayName, &c.DataType, &c.SortOrder); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan lookup master column: %w", scanErr)
		}
		out = append(out, c)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close lookup master column rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup master columns: %w", err)
	}
	return out, nil
}
