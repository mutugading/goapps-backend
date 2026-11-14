package report

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// SheetReg is the F-2 `reg` sheet: basic salary and fixed allowances per NIK, read from the frozen run detail.
const SheetReg = "reg"

// Tunjangan ids behind the reg columns (loadReg): T1 gapok, T2 jabatan, T3 rumah, T4 transport, T7 kesejahteraan,
// T55 khusus 1, T6 OT In Work, T36 Shift Adjustment.

// RegRow is one NIK of the reg sheet. Amt follows regIDs; Total = Σ of the first six (fixed income).
type RegRow struct {
	NIK, Nama, Dept, Grade, EmpType, TaxCode string
	DateJoin                                 *time.Time
	Amt                                      [8]decimal.Decimal
}

// Total is gaji pokok + the five fixed allowances (not OT In Work / Shift Adjustment).
func (r RegRow) Total() decimal.Decimal {
	t := decimal.Zero
	for _, a := range r.Amt[:6] {
		t = t.Add(a)
	}
	return t
}

var regHeader = []any{"Processing Month", hdrNIK, hdrNama, hdrDept, "Grade", "Employee Type", "Date Of Join", "Tax Code", "Gaji Pokok",
	"Tunjangan Jabatan", "Tunjangan Rumah", "Tunjangan Transport", "Tunjangan Kesejahteraan", "Tunjangan Khusus 1", "Total",
	"OT In Work", "Shift Adjustment"}

func loadReg(ctx context.Context, tx *sql.Tx, runID int64) ([]RegRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT h.psh_nik, COALESCE(h.psh_nama,''), COALESCE(h.psh_dept,''), COALESCE(h.psh_grade,''),
  COALESCE(h.psh_emp_type,''), COALESCE(h.psh_tax_code,''), h.psh_date_join,
  COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 1),0)::text, COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 2),0)::text,
  COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 3),0)::text, COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 4),0)::text,
  COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 7),0)::text, COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 55),0)::text,
  COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 6),0)::text, COALESCE(SUM(d.psd_amount) FILTER (WHERE d.psd_id = 36),0)::text
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_salary_detail d ON d.psd_run_id = h.psh_run_id AND d.psd_nik = h.psh_nik AND d.psd_type = 'Tunjangan'
  AND d.psd_id IN (1,2,3,4,7,55,6,36)
WHERE h.psh_run_id = $1
GROUP BY h.psh_run_id, h.psh_nik
ORDER BY h.psh_nik`, runID)
	if err != nil {
		return nil, fmt.Errorf("read reg: %w", err)
	}
	defer closeRows(rows)
	var out []RegRow
	for rows.Next() {
		var r RegRow
		var join sql.NullTime
		var a [8]string
		if err := rows.Scan(&r.NIK, &r.Nama, &r.Dept, &r.Grade, &r.EmpType, &r.TaxCode, &join,
			&a[0], &a[1], &a[2], &a[3], &a[4], &a[5], &a[6], &a[7]); err != nil {
			return nil, fmt.Errorf("scan reg: %w", err)
		}
		for i, s := range a {
			if r.Amt[i], err = decimal.NewFromString(s); err != nil {
				return nil, fmt.Errorf("reg amount: %w", err)
			}
		}
		if join.Valid {
			r.DateJoin = &join.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sheetWriter) reg(d RecoData) {
	s.start(SheetReg)
	s.put("Employee Basic Salary & Fixed Allowances")
	s.put()
	s.put(regHeader...)
	for _, r := range d.Reg {
		s.put(d.YYMM, r.NIK, r.Nama, optStr(r.Dept), optStr(r.Grade), optStr(r.EmpType), optDate(r.DateJoin), optStr(r.TaxCode),
			r.Amt[0], r.Amt[1], r.Amt[2], r.Amt[3], r.Amt[4], r.Amt[5], r.Total(), r.Amt[6], r.Amt[7])
	}
}

// Column headers shared by the report sheets.
const (
	hdrDept = "Dept"
	hdrNIK  = "NIK"
	hdrNama = "Nama"
)
