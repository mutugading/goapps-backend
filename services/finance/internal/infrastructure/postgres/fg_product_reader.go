package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// FgProductReader implements mbspin.FgProductReader over cost_product_master.
type FgProductReader struct {
	db *DB
}

// NewFgProductReader creates an FgProductReader.
func NewFgProductReader(db *DB) *FgProductReader { return &FgProductReader{db: db} }

var _ mbspin.FgProductReader = (*FgProductReader)(nil)

// GetFgProduct loads one product with its type code; ErrFgNotPoy when it does not exist.
func (r *FgProductReader) GetFgProduct(ctx context.Context, id int64) (*mbspin.FgProduct, error) {
	return getFgProductOn(ctx, r.db, id, false)
}

// getFgProductOn is GetFgProduct over any rowQuerier (*DB or *sql.Tx). forUpdate locks the
// product row (FOR UPDATE OF pm) and must only be used inside a transaction.
func getFgProductOn(ctx context.Context, q rowQuerier, id int64, forUpdate bool) (*mbspin.FgProduct, error) {
	var (
		p          mbspin.FgProduct
		erp, shade sql.NullString
		typeCode   sql.NullString
	)
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF pm"
	}
	err := q.QueryRowContext(ctx, `
		SELECT pm.cpm_product_sys_id, pm.cpm_product_code, pm.cpm_product_name,
		       pm.cpm_erp_item_code, pm.cpm_shade_code, pt.cpt_type_code, pm.cpm_is_active
		FROM cost_product_master pm
		LEFT JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = $1`+lock, id,
	).Scan(&p.ID, &p.Code, &p.Name, &erp, &shade, &typeCode, &p.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbspin.ErrFgNotPoy
	}
	if err != nil {
		return nil, fmt.Errorf("get fg product: %w", err)
	}
	p.ErpItemCode = nullableStringPtr(erp)
	p.ShadeCode = nullableStringPtr(shade)
	p.TypeCode = typeCode.String
	return &p, nil
}

// FillEmptyFgShade sets cpm_shade_code only while it is NULL/blank.
func (r *FgProductReader) FillEmptyFgShade(ctx context.Context, id int64, shadeCode, actor string) (bool, error) {
	return fillEmptyFgShadeOn(ctx, r.db, id, shadeCode, actor)
}

// fillEmptyFgShadeOn is FillEmptyFgShade over any rowQuerier (*DB or *sql.Tx).
func fillEmptyFgShadeOn(ctx context.Context, q rowQuerier, id int64, shadeCode, actor string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE cost_product_master
		SET cpm_shade_code = $1, cpm_updated_at = NOW(), cpm_updated_by = $2
		WHERE cpm_product_sys_id = $3 AND (cpm_shade_code IS NULL OR TRIM(cpm_shade_code) = '')`,
		shadeCode, actor, id)
	if err != nil {
		return false, fmt.Errorf("fill fg shade: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}
