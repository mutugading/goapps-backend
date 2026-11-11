package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

// MBSourceLinkReader implements mbsourceautofill.LinkReader.
type MBSourceLinkReader struct {
	db *DB
}

// NewMBSourceLinkReader creates the reader.
func NewMBSourceLinkReader(db *DB) *MBSourceLinkReader { return &MBSourceLinkReader{db: db} }

var _ mbsourceautofill.LinkReader = (*MBSourceLinkReader)(nil)

// GetLinkProduct loads the product with its type, shade, lock flag and current MB_SP_CODE text.
func (r *MBSourceLinkReader) GetLinkProduct(ctx context.Context, id int64) (mbsourceautofill.LinkProduct, error) {
	const q = `
SELECT m.cpm_product_sys_id, m.cpm_product_code, t.cpt_type_code, m.cpm_shade_code, m.cpm_is_locked,
       (SELECT c.cpp_value_text FROM cost_product_parameter c
          JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = $2 AND p.deleted_at IS NULL
         WHERE c.cpp_product_sys_id = m.cpm_product_sys_id LIMIT 1)
FROM cost_product_master m
LEFT JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
WHERE m.cpm_product_sys_id = $1`
	var (
		p                  mbsourceautofill.LinkProduct
		typeCode, shade, c sql.NullString
	)
	err := r.db.QueryRowContext(ctx, q, id, mbsource.ParamSPCode).
		Scan(&p.ProductSysID, &p.ProductCode, &typeCode, &shade, &p.Locked, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return mbsourceautofill.LinkProduct{}, mbsourceautofill.ErrLinkProductNotFound
	}
	if err != nil {
		return mbsourceautofill.LinkProduct{}, fmt.Errorf("get mb link product: %w", err)
	}
	p.TypeCode, p.Shade, p.CurrentSPCode = typeCode.String, shade.String, c.String
	return p, nil
}

// ProductCodes returns product code by id in one query.
func (r *MBSourceLinkReader) ProductCodes(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT cpm_product_sys_id, cpm_product_code
		FROM cost_product_master WHERE cpm_product_sys_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list product codes: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			id   int64
			code string
		)
		if err := rows.Scan(&id, &code); err != nil {
			return nil, fmt.Errorf("scan product code: %w", err)
		}
		out[id] = code
	}
	return out, rows.Err()
}
