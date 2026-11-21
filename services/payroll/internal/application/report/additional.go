package report

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

// SheetAdditional is the F-2 `additional` sheet: the addition rows (pyr_addition, MPA_*) of the period.
const SheetAdditional = "additional"

// AdditionRow is one pyr_addition line of the period.
type AdditionRow struct {
	NIK, Nama, Type, Name, Sts, Remarks, Division string
	ID                                            int
	Tax                                           *int
	Amount                                        decimal.Decimal
}

var additionHeader = []any{"MPA_YYMM", "MPA_NIK", "MPA_NAMA", "MPA_TYPE", "MPA_ID", "MPA_COMPNT_NAME", "MPA_COMPNT_STS",
	"MPA_COMPNT_TAX", "MPA_AMOUNT", "MPA_REMARKS", "MPA_DIVISION"}

func loadAdditions(ctx context.Context, tx *sql.Tx, q Query) ([]AdditionRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pa_nik, COALESCE(pa_nama,''), pa_type::text, pa_id, COALESCE(pa_compnt_name,''),
  COALESCE(pa_compnt_sts,''), pa_compnt_tax, pa_amount::text, COALESCE(pa_remarks,''), pa_division::text
FROM payroll.pyr_addition WHERE pa_company_id = $1 AND pa_yymm = $2 AND pa_division = $3::payroll.division
ORDER BY pa_nik, pa_type, pa_id`, q.CompanyID, q.YYMM, q.Division)
	if err != nil {
		return nil, fmt.Errorf("read additions: %w", err)
	}
	defer closeRows(rows)
	var out []AdditionRow
	for rows.Next() {
		var r AdditionRow
		var amt string
		var tax sql.NullInt64
		if err := rows.Scan(&r.NIK, &r.Nama, &r.Type, &r.ID, &r.Name, &r.Sts, &tax, &amt, &r.Remarks, &r.Division); err != nil {
			return nil, fmt.Errorf("scan addition: %w", err)
		}
		if tax.Valid {
			t := int(tax.Int64)
			r.Tax = &t
		}
		if r.Amount, err = decimal.NewFromString(amt); err != nil {
			return nil, fmt.Errorf("addition amount: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sheetWriter) additional(d RecoData) {
	s.start(SheetAdditional)
	s.put(additionHeader...)
	for _, r := range d.Additions {
		var tax any
		if r.Tax != nil {
			tax = *r.Tax
		}
		s.put(d.YYMM, r.NIK, optStr(r.Nama), r.Type, r.ID, optStr(r.Name), optStr(r.Sts), tax, r.Amount, optStr(r.Remarks), r.Division)
	}
}
