package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

// BackfillReportProduct is one product of the P6-T2 spin-move report population.
type BackfillReportProduct struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	TypeCode     string
	ShadeCode    string
	IsLocked     bool
}

// BackfillCurrentSPCode is the current MB_SP_CODE row of a product.
type BackfillCurrentSPCode struct {
	Text   string
	SpinID *uuid.UUID
}

// ListReportProducts returns the spin-move report population: active products whose
// type is not MB and whose shade is not blank, ordered by id. Read-only.
func (r *MBSpinFgBackfillRepository) ListReportProducts(ctx context.Context) ([]BackfillReportProduct, error) {
	const q = `
SELECT m.cpm_product_sys_id, m.cpm_product_code, m.cpm_product_name, COALESCE(t.cpt_type_code, ''),
       m.cpm_shade_code, m.cpm_is_locked
  FROM cost_product_master m
  LEFT JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
 WHERE m.cpm_is_active
   AND COALESCE(t.cpt_type_code, '') <> 'MB'
   AND COALESCE(TRIM(m.cpm_shade_code), '') <> ''
 ORDER BY m.cpm_product_sys_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list report products: %w", err)
	}
	defer closeRows(rows)
	var out []BackfillReportProduct
	for rows.Next() {
		var p BackfillReportProduct
		if err := rows.Scan(&p.ProductSysID, &p.ProductCode, &p.ProductName, &p.TypeCode, &p.ShadeCode, &p.IsLocked); err != nil {
			return nil, fmt.Errorf("scan report product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report products: %w", err)
	}
	return out, nil
}

// BackfillFgProductIDs returns the distinct FG product ids of live spins linked by the
// backfill (mbs_fg_link_source = BACKFILL). Read-only.
func (r *MBSpinFgBackfillRepository) BackfillFgProductIDs(ctx context.Context) ([]int64, error) {
	const q = `
SELECT DISTINCT mbs_fg_product_id
  FROM mst_mb_spin
 WHERE mbs_fg_link_source = 'BACKFILL' AND mbs_fg_product_id IS NOT NULL
   AND deleted_at IS NULL AND mbs_is_active AND mbs_superseded_by_spin_id IS NULL
 ORDER BY mbs_fg_product_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list backfill fg products: %w", err)
	}
	defer closeRows(rows)
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan backfill fg product: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backfill fg products: %w", err)
	}
	return out, nil
}

// ActivePoyShades returns, per id in ids that is an active POY product, its normalized
// (UPPER(TRIM)) shade ("" when blank). Non-POY / inactive / unknown ids are absent. Read-only.
func (r *MBSpinFgBackfillRepository) ActivePoyShades(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
SELECT p.cpm_product_sys_id, COALESCE(UPPER(TRIM(p.cpm_shade_code)), '')
  FROM cost_product_master p
  JOIN cost_product_type t ON t.cpt_type_id = p.cpm_product_type_id
 WHERE p.cpm_product_sys_id = ANY($1) AND t.cpt_type_code = 'POY' AND p.cpm_is_active`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list active poy shades: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			id    int64
			shade string
		)
		if err := rows.Scan(&id, &shade); err != nil {
			return nil, fmt.Errorf("scan active poy shade: %w", err)
		}
		out[id] = shade
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active poy shades: %w", err)
	}
	return out, nil
}

// CurrentSPCodes returns the current MB_SP_CODE value (text + spin pointer) per product
// that has a cost_product_parameter row for it. Read-only.
func (r *MBSpinFgBackfillRepository) CurrentSPCodes(ctx context.Context, ids []int64) (map[int64]BackfillCurrentSPCode, error) {
	out := make(map[int64]BackfillCurrentSPCode, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
SELECT c.cpp_product_sys_id, COALESCE(c.cpp_value_text, ''), c.cpp_value_mb_spin_id
  FROM cost_product_parameter c
  JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = $2 AND p.deleted_at IS NULL
 WHERE c.cpp_product_sys_id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids), mbsource.ParamSPCode)
	if err != nil {
		return nil, fmt.Errorf("list current mb sp codes: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			id     int64
			cur    BackfillCurrentSPCode
			spinID uuid.NullUUID
		)
		if err := rows.Scan(&id, &cur.Text, &spinID); err != nil {
			return nil, fmt.Errorf("scan current mb sp code: %w", err)
		}
		if spinID.Valid {
			v := spinID.UUID
			cur.SpinID = &v
		}
		out[id] = cur
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current mb sp codes: %w", err)
	}
	return out, nil
}
